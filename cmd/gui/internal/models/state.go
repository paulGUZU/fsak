package models

import (
	"sync"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"github.com/paulGUZU/fsak/internal/client"
)

// ConnectionMode represents the connection mode
type ConnectionMode string

const (
	ModeProxy ConnectionMode = "proxy"
	ModeTUN   ConnectionMode = "tun"
)

// ConnectionStatus represents the current connection state
type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusDisconnecting
)

func (s ConnectionStatus) String() string {
	switch s {
	case StatusConnecting:
		return "Connecting"
	case StatusConnected:
		return "Connected"
	case StatusDisconnecting:
		return "Disconnecting"
	default:
		return "Disconnected"
	}
}

// RunningClient holds the runtime state of an active connection
type RunningClient struct {
	ProfileName string
	Mode        ConnectionMode
	Pool        *client.AddressPool
	SOCKS       *client.SOCKS5Server
	SystemProxy client.SystemProxySession
	Done        chan error
	StartedAt   time.Time
	CleanupMu   sync.Mutex
	CleanedUp   bool
}

// Cleanup performs cleanup of the running client
func (r *RunningClient) Cleanup(timeout time.Duration) error {
	r.CleanupMu.Lock()
	defer r.CleanupMu.Unlock()
	if r.CleanedUp {
		return nil
	}

	var firstErr error
	if r.SystemProxy != nil {
		if err := r.SystemProxy.Disable(); err != nil {
			firstErr = err
		} else {
			r.SystemProxy = nil
		}
	}

	// SOCKS and Pool cleanup handled by context
	if firstErr == nil {
		r.CleanedUp = true
	}
	return firstErr
}

// GUIState holds the reactive application state
type GUIState struct {
	mu sync.RWMutex

	// Profiles
	profiles map[string]ClientConfig
	selected string

	// Runtime
	runner  *RunningClient
	lastErr string

	// Bindings for reactive UI updates
	SelectedProfile binding.String
	ConnectionState binding.Int // ConnectionStatus
	LastError       binding.String
	ProfileList     binding.StringList

	// Callbacks for state changes
	onProfileChanged func(name string)
	onStateChanged   func(status ConnectionStatus)
}

// NewGUIState creates a new GUI state with bindings
func NewGUIState() *GUIState {
	s := &GUIState{
		profiles:        make(map[string]ClientConfig),
		SelectedProfile: binding.NewString(),
		ConnectionState: binding.NewInt(),
		LastError:       binding.NewString(),
		ProfileList:     binding.NewStringList(),
	}
	return s
}

// SetProfileChangedCallback sets the callback for profile changes
func (s *GUIState) SetProfileChangedCallback(cb func(name string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProfileChanged = cb
}

// SetStateChangedCallback sets the callback for connection state changes
func (s *GUIState) SetStateChangedCallback(cb func(status ConnectionStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStateChanged = cb
}

// Profiles returns a copy of the profiles map
func (s *GUIState) Profiles() map[string]ClientConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cloned := make(map[string]ClientConfig, len(s.profiles))
	for k, v := range s.profiles {
		cloned[k] = v
	}
	return cloned
}

// ProfileNames returns sorted profile names
func (s *GUIState) ProfileNames() []string {
	return SortedProfileNames(s.Profiles())
}

// GetProfile returns a profile by name
func (s *GUIState) GetProfile(name string) (ClientConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.profiles[name]
	return cfg, ok
}

// SetProfile adds or updates a profile
func (s *GUIState) SetProfile(name string, cfg ClientConfig) {
	s.mu.Lock()
	s.profiles[name] = cfg
	s.mu.Unlock()

	s.updateProfileList()
	if s.onProfileChanged != nil {
		s.onProfileChanged(name)
	}
}

// DeleteProfile removes a profile
func (s *GUIState) DeleteProfile(name string) bool {
	s.mu.Lock()
	deleted := false
	if _, exists := s.profiles[name]; exists {
		delete(s.profiles, name)
		deleted = true
		if s.selected == name {
			if len(s.profiles) > 0 {
				s.selected = SortedProfileNames(s.profiles)[0]
			} else {
				s.selected = ""
			}
		}
	}
	s.mu.Unlock()

	if deleted {
		s.updateProfileList()
		s.SelectedProfile.Set(s.selected)
		if s.onProfileChanged != nil {
			s.onProfileChanged(s.selected)
		}
	}
	return deleted
}

// Selected returns the currently selected profile name
func (s *GUIState) Selected() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected
}

// SetSelected sets the selected profile
func (s *GUIState) SetSelected(name string) {
	s.mu.Lock()
	s.selected = name
	s.mu.Unlock()

	s.SelectedProfile.Set(name)
	if s.onProfileChanged != nil {
		s.onProfileChanged(name)
	}
}

// Runner returns the current running client
func (s *GUIState) Runner() *RunningClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runner
}

// SetRunner sets the running client
func (s *GUIState) SetRunner(r *RunningClient) {
	s.mu.Lock()
	s.runner = r
	status := StatusDisconnected
	if r != nil {
		status = StatusConnected
	}
	s.mu.Unlock()

	s.ConnectionState.Set(int(status))
	if s.onStateChanged != nil {
		s.onStateChanged(status)
	}
}

// ClearRunner clears the running client if it matches
func (s *GUIState) ClearRunner(r *RunningClient) bool {
	s.mu.Lock()
	cleared := false
	if s.runner == r {
		s.runner = nil
		cleared = true
	}
	s.mu.Unlock()

	if cleared {
		s.ConnectionState.Set(int(StatusDisconnected))
		if s.onStateChanged != nil {
			s.onStateChanged(StatusDisconnected)
		}
	}
	return cleared
}

// IsRunning returns true if a client is running
func (s *GUIState) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runner != nil
}

// RunningProfile returns the name of the running profile
func (s *GUIState) RunningProfile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.runner != nil {
		return s.runner.ProfileName
	}
	return ""
}

// SetError sets the last error message
func (s *GUIState) SetError(err string) {
	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
	s.LastError.Set(err)
}

// ClearError clears the last error
func (s *GUIState) ClearError() {
	s.SetError("")
}

// GetError returns the last error
func (s *GUIState) GetError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}

// SelectedConfig returns the selected profile's config
func (s *GUIState) SelectedConfig() (string, ClientConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.selected == "" {
		return "", ClientConfig{}, false
	}
	cfg, ok := s.profiles[s.selected]
	return s.selected, cfg, ok
}

// updateProfileList updates the binding with current profile names
func (s *GUIState) updateProfileList() {
	names := s.ProfileNames()
	s.ProfileList.Set(names)
}

// InitializeProfileList initializes the profile list binding
func (s *GUIState) InitializeProfileList() {
	s.updateProfileList()
	s.SelectedProfile.Set(s.selected)
}

// ReplaceProfiles replaces all profiles (used after loading)
func (s *GUIState) ReplaceProfiles(profiles map[string]ClientConfig, selected string) {
	s.mu.Lock()
	s.profiles = profiles
	s.selected = selected
	s.mu.Unlock()

	s.updateProfileList()
	s.SelectedProfile.Set(selected)
}
