package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/paulGUZU/fsak/pkg/config"
)

// ProfileService handles profile persistence
type ProfileService struct {
	storePath string
}

// NewProfileService creates a new profile service
func NewProfileService(storePath string) *ProfileService {
	return &ProfileService{storePath: storePath}
}

// DefaultStorePath returns the default profile storage path
func DefaultStorePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, models.ConfigDirName, models.ProfilesFileName), nil
}

// LoadProfiles loads profiles from storage
func (s *ProfileService) LoadProfiles() (map[string]models.ClientConfig, string, error) {
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.seedDefaultProfile()
		}
		return nil, "", err
	}

	var file models.ProfilesStore
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, "", err
	}

	profiles := make(map[string]models.ClientConfig)
	for _, p := range file.Profiles {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		cfg, err := p.Config.Normalize()
		if err != nil {
			continue
		}
		profiles[name] = cfg
	}

	if len(profiles) == 0 {
		return s.seedDefaultProfile()
	}

	selected := file.Selected
	if _, ok := profiles[selected]; !ok {
		selected = sortedNames(profiles)[0]
	}

	return profiles, selected, nil
}

// SaveProfiles saves profiles to storage
func (s *ProfileService) SaveProfiles(selected string, profiles map[string]models.ClientConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.storePath), 0o755); err != nil {
		return err
	}

	names := sortedNames(profiles)
	profileList := make([]models.ClientProfile, 0, len(names))
	for _, name := range names {
		profileList = append(profileList, models.ClientProfile{
			Name:   name,
			Config: profiles[name],
		})
	}

	payload, err := json.MarshalIndent(models.ProfilesStore{
		Selected: selected,
		Profiles: profileList,
	}, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write
	tmp := s.storePath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.storePath)
}

// seedDefaultProfile creates a default profile
func (s *ProfileService) seedDefaultProfile() (map[string]models.ClientConfig, string, error) {
	profiles := make(map[string]models.ClientConfig)

	// Try to load from config.json
	if cfg, err := config.LoadConfig("config.json"); err == nil {
		profiles["default"] = models.ClientConfigFromInternal(*cfg)
	} else {
		profiles["default"] = models.ClientConfig{
			Addresses: []string{"127.0.0.1"},
			Host:      "localhost",
			TLS:       false,
			SNI:       "",
			Port:      8080,
			ProxyPort: 1080,
			Secret:    "",
		}
	}

	selected := "default"
	if err := s.SaveProfiles(selected, profiles); err != nil {
		return profiles, selected, err
	}
	return profiles, selected, nil
}

func sortedNames(profiles map[string]models.ClientConfig) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
