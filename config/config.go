package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

type Config struct {
	Host     string   `json:"host"`
	Port     uint     `json:"port"`
	Logger   bool     `json:"logger"`
	ReadOnly bool     `json:"read_only"`
	Room     uint     `json:"room"`
	Tor      bool     `json:"tor"`
	UserID   int      `json:"user_id"`
	Args     []string `json:",omitempty"`
}

func LoadConfig() (Config, error) {
	// Generate Config from template.
	// JSON decode will set any existing values.
	// New keys get written with defaults.
	cfg := newConfig()

	cfgDir, err := getConfigDir()
	if err != nil {
		return cfg, err
	}
	// Create config dir if necessary.
	err = os.Mkdir(cfgDir, 0755)
	// If the dir already exists, it will return an fs.ErrExist error.
	// In that case, we just want to continue without returning early.
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return cfg, err
	}

	// Open config.json file for RW.
	f, err := os.OpenFile(fmt.Sprintf("%s/config.json", cfgDir), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	jd := json.NewDecoder(f)
	for jd.More() {
		if err := jd.Decode(&cfg); err != nil {
			return cfg, err
		}
	}

	// Truncate and write config with any potential new keys.
	if err := f.Truncate(0); err != nil {
		return cfg, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return cfg, err
	}
	if err := writeConfig(f, cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// Get path string of user config dir.
func getConfigDir() (string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/sockchat", ucd), nil
}

func newConfig() Config {
	return Config{
		Host:     "kiwifarms.net",
		Port:     9443,
		Logger:   false,
		ReadOnly: false,
		Room:     1,
		Tor:      false,
		UserID:   -1,
	}
}

func writeConfig(f *os.File, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	if _, err := f.Write(b); err != nil {
		return err
	}
	// Write trailing newline to file.
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	return nil
}
