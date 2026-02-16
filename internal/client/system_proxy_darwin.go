//go:build darwin

package client

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type socksProxyState struct {
	enabled bool
	server  string
	port    int
}

type darwinSystemProxySession struct {
	services []string
	previous map[string]socksProxyState
}

func EnableSystemProxy(port int) (SystemProxySession, error) {
	services, err := listNetworkServices()
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no active macOS network services found")
	}

	previous := make(map[string]socksProxyState, len(services))
	changed := make([]string, 0, len(services))

	for _, service := range services {
		state, err := getSOCKSProxyState(service)
		if err != nil {
			rollbackErr := rollbackServices(changed, previous)
			if rollbackErr != nil {
				return nil, fmt.Errorf("failed on service %q: %v (rollback failed: %v)", service, err, rollbackErr)
			}
			return nil, fmt.Errorf("failed reading proxy state for %q: %w", service, err)
		}
		previous[service] = state

		if err := runNetworkSetup("-setsocksfirewallproxy", service, "127.0.0.1", strconv.Itoa(port)); err != nil {
			rollbackErr := rollbackServices(changed, previous)
			if rollbackErr != nil {
				return nil, fmt.Errorf("failed enabling SOCKS for %q: %v (rollback failed: %v)", service, err, rollbackErr)
			}
			return nil, fmt.Errorf("failed enabling SOCKS for %q: %w", service, err)
		}
		if err := runNetworkSetup("-setsocksfirewallproxystate", service, "on"); err != nil {
			rollbackErr := rollbackServices(changed, previous)
			if rollbackErr != nil {
				return nil, fmt.Errorf("failed enabling SOCKS state for %q: %v (rollback failed: %v)", service, err, rollbackErr)
			}
			return nil, fmt.Errorf("failed enabling SOCKS state for %q: %w", service, err)
		}
		changed = append(changed, service)
	}

	return &darwinSystemProxySession{
		services: services,
		previous: previous,
	}, nil
}

func (s *darwinSystemProxySession) Disable() error {
	return rollbackServices(s.services, s.previous)
}

func rollbackServices(services []string, previous map[string]socksProxyState) error {
	var errs []string
	for _, service := range services {
		state, ok := previous[service]
		if !ok {
			continue
		}
		if state.enabled {
			port := state.port
			if port <= 0 {
				port = 1080
			}
			server := state.server
			if strings.TrimSpace(server) == "" {
				server = "127.0.0.1"
			}
			if err := runNetworkSetup("-setsocksfirewallproxy", service, server, strconv.Itoa(port)); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", service, err))
				continue
			}
			if err := runNetworkSetup("-setsocksfirewallproxystate", service, "on"); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", service, err))
			}
			continue
		}
		if err := runNetworkSetup("-setsocksfirewallproxystate", service, "off"); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", service, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to restore macOS SOCKS proxy on services: %s", strings.Join(errs, "; "))
	}
	return nil
}

func listNetworkServices() ([]string, error) {
	out, err := runNetworkSetupOutput("-listallnetworkservices")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out, "\n")
	services := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") {
			continue
		}
		if strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

func getSOCKSProxyState(service string) (socksProxyState, error) {
	out, err := runNetworkSetupOutput("-getsocksfirewallproxy", service)
	if err != nil {
		return socksProxyState{}, err
	}

	state := socksProxyState{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Enabled":
			state.enabled = strings.EqualFold(val, "Yes")
		case "Server":
			state.server = val
		case "Port":
			port, _ := strconv.Atoi(val)
			state.port = port
		}
	}

	return state, nil
}

func runNetworkSetup(args ...string) error {
	_, err := runNetworkSetupOutput(args...)
	return err
}

func runNetworkSetupOutput(args ...string) (string, error) {
	cmd := exec.Command("networksetup", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("networksetup %s failed: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
