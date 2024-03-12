package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config object with set parameters.
// Using JSON because it's hard to fuck up the formatting unless you're braindead.
// I like YAML, but it requires more caution than I can expect from most users.
type Config struct {
	Host     string `json:"host"`
	Port     uint   `json:"port"`
	Logger   bool   `json:"logger"`
	ReadOnly bool   `json:"read_only"`
	Room     uint   `json:"room"`
	Tor      bool   `json:"tor"`
	UserID   int    `json:"user_id"`

	ApiMode bool `json:",omitempty"`
	// Used for collecting remaining args, containing cookies.
	Args []string `json:",omitempty"`
}

// Load user config from config.json file in the UserConfigDir provided by the os package.
// Creates the file and fills it with defaults from newConfig if it isn't found.
// Adds missing option keys, if any, with defaults to file.
func LoadConfig() (Config, error) {
	// Generate Config from template.
	// JSON decode will set any existing values.
	// New keys get written with defaults.
	cfg := newConfig()

	cfgDir, err := configDir()
	if err != nil {
		return cfg, err
	}

	// Open config.json file for RW.
	f, err := os.OpenFile(fmt.Sprintf("%s/config.json", cfgDir), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	// Decode JSON from file descriptor.
	jd := json.NewDecoder(f)
	for jd.More() {
		if err := jd.Decode(&cfg); err != nil {
			return cfg, err
		}
	}

	// Truncate and write loaded config with any potential new keys.
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
func configDir() (string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	cfgDir := fmt.Sprintf("%s/sockchat", ucd)

	// Create config dir if necessary. Doesn't break anything existing.
	// If the dir already exists, it will return an fs.ErrExist error.
	// In that case, we just want to continue without returning early.
	if err := os.Mkdir(cfgDir, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}

	return cfgDir, nil
}

// Generate new Config from template.
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

// Write loaded config to config.json file.
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
