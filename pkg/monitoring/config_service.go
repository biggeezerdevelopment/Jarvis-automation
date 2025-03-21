package monitoring

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigService handles configuration management
type ConfigService struct {
	configPath string
	monitor    *Monitor
	mu         sync.RWMutex
}

// NewConfigService creates a new configuration service
func NewConfigService(configPath string, monitor *Monitor) *ConfigService {
	return &ConfigService{
		configPath: configPath,
		monitor:    monitor,
	}
}

// LoadConfig loads configuration from file
func (s *ConfigService) LoadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := ioutil.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %v", err)
	}

	if err := s.monitor.UpdateConfig(&config); err != nil {
		return fmt.Errorf("failed to update monitor config: %v", err)
	}

	return nil
}

// SaveConfig saves current configuration to file
func (s *ConfigService) SaveConfig() error {
	s.mu.RLock()
	config := s.monitor.GetConfig()
	s.mu.RUnlock()

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Create backup of current config
	backupPath := s.configPath + ".bak"
	if err := os.Rename(s.configPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %v", err)
	}

	// Write new config
	if err := ioutil.WriteFile(s.configPath, data, 0644); err != nil {
		// Restore backup on error
		os.Rename(backupPath, s.configPath)
		return fmt.Errorf("failed to write config: %v", err)
	}

	// Remove backup on success
	os.Remove(backupPath)
	return nil
}

// UpdateConfig updates configuration with new values
func (s *ConfigService) UpdateConfig(newConfig *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.monitor.UpdateConfig(newConfig); err != nil {
		return err
	}

	return s.SaveConfig()
}

// GetConfig returns current configuration
func (s *ConfigService) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.monitor.GetConfig()
}

// WatchConfig watches for configuration file changes
func (s *ConfigService) WatchConfig() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			if err := s.LoadConfig(); err != nil {
				fmt.Printf("Error reloading config: %v\n", err)
			}
		}
	}()
}
