package services

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/xjasonlyu/tun2socks/v2/engine"
)

// TUNSession represents an active TUN session
type TUNSession struct {
	process *os.Process
	done    chan error
	logs    *cappedBuffer
}

// Disable stops the TUN session
func (s *TUNSession) Disable() error {
	if s == nil {
		return nil
	}

	if s.process != nil {
		_ = s.process.Signal(syscall.SIGTERM)
	}

	select {
	case <-s.done:
		return nil
	case <-time.After(models.TunStopTimeout):
		if s.process != nil {
			_ = s.process.Kill()
		}
		<-s.done
		return nil
	}
}

// Done returns the done channel
func (s *TUNSession) Done() <-chan error {
	if s == nil {
		return nil
	}
	return s.done
}

// cappedBuffer is a thread-safe bounded buffer for logs
type cappedBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.max <= 0 {
		b.max = models.MaxLogBuffer
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// StartTUNSession starts a TUN session
func StartTUNSession(proxyPort int, bindInterface string, bypassEntries []string) (*TUNSession, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("TUN mode is only supported on macOS")
	}

	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve executable path: %w", err)
	}

	args := []string{models.TunHelperArg, "--proxy-port", strconv.Itoa(proxyPort)}
	if strings.TrimSpace(bindInterface) != "" {
		args = append(args, "--interface", strings.TrimSpace(bindInterface))
	}
	if len(bypassEntries) > 0 {
		args = append(args, "--bypass", strings.Join(bypassEntries, ","))
	}

	cmd := exec.Command(exePath, args...)
	logs := &cappedBuffer{max: models.MaxLogBuffer}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start TUN helper: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for startup
	select {
	case err := <-done:
		msg := strings.TrimSpace(logs.String())
		if err == nil {
			if msg != "" {
				return nil, fmt.Errorf("TUN helper exited unexpectedly: %s", msg)
			}
			return nil, errors.New("TUN helper exited unexpectedly")
		}
		if msg != "" {
			return nil, fmt.Errorf("TUN helper failed: %v (%s)", err, msg)
		}
		return nil, fmt.Errorf("TUN helper failed: %w", err)
	case <-time.After(models.TunStartupTimeout):
	}

	return &TUNSession{
		process: cmd.Process,
		done:    done,
		logs:    logs,
	}, nil
}

// RunTUNHelper runs the TUN helper process (called with --fsak-tun-helper)
func RunTUNHelper(args []string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("TUN helper is only supported on macOS")
	}

	var proxyPort int
	var tunDevice string
	var bindInterface string
	var bypassRaw string

	// Parse flags
	fs := createFlagSet()
	fs.IntVar(&proxyPort, "proxy-port", 0, "local SOCKS5 port")
	fs.StringVar(&tunDevice, "device", models.TunDevice, "TUN device name")
	fs.StringVar(&bindInterface, "interface", "", "physical egress interface")
	fs.StringVar(&bypassRaw, "bypass", "", "comma separated server IPs/CIDRs to bypass")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if proxyPort < 1 || proxyPort > 65535 {
		return errors.New("invalid proxy-port for TUN helper")
	}

	// Detect default route
	defaultIface, defaultGateway, err := detectDefaultRouteDarwin()
	if err != nil {
		return fmt.Errorf("failed to detect default route: %w", err)
	}
	if bindInterface == "" {
		bindInterface = defaultIface
	}
	if strings.TrimSpace(defaultGateway) == "" {
		return errors.New("default gateway not found for TUN setup")
	}

	bypassEntries := splitBypassEntries(bypassRaw)

	// Start tun2socks engine
	key := &engine.Key{
		MTU:       1500,
		Proxy:     fmt.Sprintf("socks5://127.0.0.1:%d", proxyPort),
		Device:    tunDevice,
		Interface: bindInterface,
		LogLevel:  "warn",
	}
	engine.Insert(key)
	engine.Start()
	defer engine.Stop()

	// Setup routes
	cleanup, err := setupDarwinTunnelRoutes(tunDevice, defaultGateway, bypassEntries)
	if err != nil {
		return fmt.Errorf("failed to configure tunnel routes: %w", err)
	}
	defer func() { _ = cleanup() }()

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh

	return nil
}

func createFlagSet() *flagSet {
	return &flagSet{}
}

type flagSet struct {
	intVars    map[string]*int
	stringVars map[string]*string
}

func (f *flagSet) IntVar(p *int, name string, value int, usage string) {
	*p = value
	if f.intVars == nil {
		f.intVars = make(map[string]*int)
	}
	f.intVars[name] = p
}

func (f *flagSet) StringVar(p *string, name string, value string, usage string) {
	*p = value
	if f.stringVars == nil {
		f.stringVars = make(map[string]*string)
	}
	f.stringVars[name] = p
}

func (f *flagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if i+1 >= len(args) {
			continue
		}
		value := args[i+1]

		if p, ok := f.intVars[name]; ok {
			if v, err := strconv.Atoi(value); err == nil {
				*p = v
			}
			i++
		}
		if p, ok := f.stringVars[name]; ok {
			*p = value
			i++
		}
	}
	return nil
}

func detectDefaultRouteDarwin() (iface string, gateway string, err error) {
	out, err := runCommand("route", "-n", "get", "default")
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
		if strings.HasPrefix(line, "gateway:") {
			gateway = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		}
	}
	if iface == "" {
		return "", "", errors.New("default interface not found in route output")
	}
	if gateway == "" {
		return "", "", errors.New("default gateway not found in route output")
	}
	return iface, gateway, nil
}

func setupDarwinTunnelRoutes(tunDevice string, defaultGateway string, bypassEntries []string) (func() error, error) {
	if err := runCommandErr("ifconfig", tunDevice, "inet", "198.18.0.1", "198.18.0.1", "up"); err != nil {
		return nil, fmt.Errorf("ifconfig %s up failed (run GUI with elevated privileges): %w", tunDevice, err)
	}

	bypassRoutes := collectBypassRoutes(bypassEntries)
	for _, target := range bypassRoutes {
		_ = runCommandErr("route", "-n", "delete", target.kindFlag, target.value)
		if err := runCommandErr("route", "-n", "add", target.kindFlag, target.value, defaultGateway); err != nil {
			return nil, fmt.Errorf("failed to add bypass route %s %s via %s: %w", target.kindFlag, target.value, defaultGateway, err)
		}
	}

	if err := replaceDarwinSplitRoute("0.0.0.0/1", tunDevice); err != nil {
		return nil, err
	}
	if err := replaceDarwinSplitRoute("128.0.0.0/1", tunDevice); err != nil {
		return nil, err
	}

	return func() error {
		var errs []string
		if err := runCommandErr("route", "-n", "delete", "-net", "0.0.0.0/1", "-interface", tunDevice); err != nil {
			errs = append(errs, err.Error())
		}
		if err := runCommandErr("route", "-n", "delete", "-net", "128.0.0.0/1", "-interface", tunDevice); err != nil {
			errs = append(errs, err.Error())
		}
		for _, target := range bypassRoutes {
			if err := runCommandErr("route", "-n", "delete", target.kindFlag, target.value); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if err := runCommandErr("ifconfig", tunDevice, "down"); err != nil {
			errs = append(errs, err.Error())
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	}, nil
}

func replaceDarwinSplitRoute(cidr string, tunDevice string) error {
	_ = runCommandErr("route", "-n", "delete", "-net", cidr, "-interface", tunDevice)
	if err := runCommandErr("route", "-n", "add", "-net", cidr, "-interface", tunDevice); err != nil {
		return fmt.Errorf("route add %s via %s failed: %w", cidr, tunDevice, err)
	}
	return nil
}

type bypassRoute struct {
	kindFlag string
	value    string
}

func collectBypassRoutes(entries []string) []bypassRoute {
	seen := make(map[string]struct{})
	routes := make([]bypassRoute, 0, len(entries))

	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "-") {
			continue // IP range syntax not supported
		}

		if _, ipNet, err := net.ParseCIDR(raw); err == nil {
			key := "-net|" + ipNet.String()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, bypassRoute{kindFlag: "-net", value: ipNet.String()})
			continue
		}

		if ip := net.ParseIP(raw); ip != nil {
			ipStr := ip.String()
			key := "-host|" + ipStr
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, bypassRoute{kindFlag: "-host", value: ipStr})
		}
	}

	return routes
}

func splitBypassEntries(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), trimmed)
	}
	return trimmed, nil
}

func runCommandErr(name string, args ...string) error {
	_, err := runCommand(name, args...)
	return err
}

// Ensure imports are used
var _ = bufio.NewReader
var _ = io.ReadFull
