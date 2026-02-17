package services

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/paulGUZU/fsak/internal/client"
)

// RunnerService manages the connection lifecycle
type RunnerService struct {
	state *models.GUIState
}

// NewRunnerService creates a new runner service
func NewRunnerService(state *models.GUIState) *RunnerService {
	return &RunnerService{state: state}
}

// StartOptions contains options for starting a connection
type StartOptions struct {
	ProfileName string
	Config      models.ClientConfig
	Mode        models.ConnectionMode
}

// Start begins a new connection
func (s *RunnerService) Start(opts StartOptions) error {
	if s.state.IsRunning() {
		return errors.New("client is already running")
	}

	internalCfg := opts.Config.ToInternal()

	// Create address pool
	pool, err := client.NewAddressPool(internalCfg.Addresses, internalCfg.Port, internalCfg.Host, internalCfg.TLS)
	if err != nil {
		return fmt.Errorf("failed to create address pool: %w", err)
	}

	// Create transport and SOCKS server
	transport := client.NewTransport(&internalCfg, pool)
	socks := client.NewSOCKS5Server(internalCfg.ProxyPort, transport)
	socksDone := make(chan error, 1)

	go func() {
		socksDone <- socks.ListenAndServe()
	}()

	// Wait for SOCKS to start
	select {
	case err := <-socksDone:
		pool.Stop()
		if err == nil {
			return errors.New("SOCKS server stopped unexpectedly")
		}
		return fmt.Errorf("SOCKS server failed to start: %w", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Handle system proxy setup
	var systemProxy client.SystemProxySession
	var systemDone <-chan error

	if opts.Mode == models.ModeTUN {
		// TUN mode - only supported on macOS
		if runtime.GOOS != "darwin" {
			ctx, cancel := context.WithTimeout(context.Background(), models.ConnectionTimeout)
			defer cancel()
			_ = socks.Stop(ctx)
			pool.Stop()
			return errors.New("TUN mode is only supported on macOS")
		}

		tunSession, err := StartTUNSession(internalCfg.ProxyPort, "", internalCfg.Addresses)
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), models.ConnectionTimeout)
			defer cancel()
			_ = socks.Stop(ctx)
			pool.Stop()
			return fmt.Errorf("failed to start TUN runtime: %w", err)
		}
		systemProxy = tunSession
		systemDone = tunSession.Done()
	} else {
		// Proxy mode - enable system proxy on all supported platforms
		proxySession, err := client.EnableSystemProxy(internalCfg.ProxyPort)
		if err != nil {
			// Log warning but continue - system proxy is optional
			fmt.Printf("Warning: failed to set system proxy: %v\n", err)
		} else {
			systemProxy = proxySession
		}
	}

	// Create done channel
	done := make(chan error, 1)
	go func() {
		if systemDone == nil {
			done <- <-socksDone
			return
		}
		select {
		case err := <-socksDone:
			done <- err
		case err := <-systemDone:
			if err == nil {
				err = errors.New("TUN runtime exited unexpectedly")
			}
			done <- err
		}
	}()

	// Set runner
	runner := &models.RunningClient{
		ProfileName: opts.ProfileName,
		Mode:        opts.Mode,
		Pool:        pool,
		SOCKS:       socks,
		SystemProxy: systemProxy,
		Done:        done,
		StartedAt:   time.Now(),
	}

	s.state.SetRunner(runner)
	s.state.ClearError()

	return nil
}

// Stop stops the current connection
func (s *RunnerService) Stop() error {
	runner := s.state.Runner()
	if runner == nil {
		return nil
	}

	// Try graceful shutdown first
	if err := runner.Cleanup(models.ConnectionTimeout); err != nil {
		// If timeout, try with longer timeout
		if errors.Is(err, context.DeadlineExceeded) {
			if err := runner.Cleanup(models.ConnectionRetryTimeout); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Stop SOCKS server
	if runner.SOCKS != nil {
		ctx, cancel := context.WithTimeout(context.Background(), models.ConnectionTimeout)
		defer cancel()
		if err := runner.SOCKS.Stop(ctx); err != nil {
			// Non-fatal, continue cleanup
		}
	}

	// Stop address pool
	if runner.Pool != nil {
		runner.Pool.Stop()
	}

	s.state.ClearRunner(runner)
	return nil
}

// ForceStop forces an immediate stop
func (s *RunnerService) ForceStop() error {
	runner := s.state.Runner()
	if runner == nil {
		return nil
	}

	// Quick cleanup
	_ = runner.Cleanup(1 * time.Second)

	if runner.SOCKS != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = runner.SOCKS.Stop(ctx)
	}

	if runner.Pool != nil {
		runner.Pool.Stop()
	}

	s.state.ClearRunner(runner)
	return nil
}

// Watch monitors the runner and handles disconnection
func (s *RunnerService) Watch(onDisconnect func(error)) {
	runner := s.state.Runner()
	if runner == nil {
		return
	}

	// Wait for done signal
	go func() {
		err := <-runner.Done

		// Cleanup
		_ = runner.Cleanup(models.ConnectionTimeout)

		// Clear runner if still the same
		if s.state.ClearRunner(runner) {
			if err != nil {
				s.state.SetError(err.Error())
			}
			if onDisconnect != nil {
				onDisconnect(err)
			}
		}
	}()
}

// Status returns current connection status
func (s *RunnerService) Status() (connected bool, profile string, mode models.ConnectionMode, started time.Time) {
	runner := s.state.Runner()
	if runner == nil {
		return false, "", "", time.Time{}
	}
	return true, runner.ProfileName, runner.Mode, runner.StartedAt
}
