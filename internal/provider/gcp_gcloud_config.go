package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/ini.v1"
)

// loadActiveGcloudConfigINI loads the INI file for the active gcloud configuration
// (same resolution as the gcloud CLI: active_config → configurations/config_<name>).
func loadActiveGcloudConfigINI() (*ini.File, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	var gcloudConfigDir string
	if runtime.GOOS == "windows" {
		gcloudConfigDir = filepath.Join(homeDir, "AppData", "Roaming", "gcloud")
	} else {
		gcloudConfigDir = filepath.Join(homeDir, ".config", "gcloud")
	}

	activeConfig := "default"
	activeConfigPath := filepath.Join(gcloudConfigDir, "active_config")
	if activeConfigData, err := os.ReadFile(activeConfigPath); err == nil {
		activeConfig = strings.TrimSpace(string(activeConfigData))
	}

	configFilePath := filepath.Join(gcloudConfigDir, "configurations", fmt.Sprintf("config_%s", activeConfig))

	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		configFilePath = filepath.Join(gcloudConfigDir, "properties")
		if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("gcloud config file not found at %s or properties file", configFilePath)
		}
	}

	cfg, err := ini.Load(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read gcloud config file: %w", err)
	}

	return cfg, nil
}

// readActiveGcloudAccount returns core.account from the active gcloud configuration.
// It returns an empty string if the config cannot be read or account is unset.
func readActiveGcloudAccount() string {
	cfg, err := loadActiveGcloudConfigINI()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Section("core").Key("account").String())
}
