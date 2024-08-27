package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config object with set parameters.
// Using JSON because it's hard to fuck up the formatting unless you're braindead.
// I like YAML, but it requires more caution than I can expect from most users.
type Config struct {
	Cookies  string `json:"cookies"`
	Host     string `json:"host"`
	Port     uint   `json:"port"`
	Logger   bool   `json:"logger"`
	ReadOnly bool   `json:"read_only"`
	Room     uint   `json:"room"`
	Tor      bool   `json:"tor"`
	UserID   int    `json:"user_id"`

	Proxy proxyConfig `json:"proxy"`

	// ApiMode bool `json:",omitempty"`

	// Used for collecting remaining args, containing cookies.
	Args []string `json:",omitempty"`
}

type proxyConfig struct {
	Enabled bool   `json:"enabled"`
	Addr    string `json:"address"`
	User    string `json:"username"`
	Pass    string `json:"password"`
}

func openConfig() (*os.File, error) {
	cfgDir, err := configDir()
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(cfgDir, "config.json")

	return os.OpenFile(cfgPath, os.O_CREATE|os.O_RDWR, 0644)
}

// Load user config from config.json file in the UserConfigDir provided by the os package.
// Creates the file and fills it with defaults from newConfig if it isn't found.
// Adds missing option keys, if any, with defaults to file.
func LoadConfig() (cfg Config, err error) {
	// Generate Config from template.
	// JSON decode will set any existing values.
	// New keys get set to defaults and saved.
	cfg = newConfig()

	f, err := openConfig()
	if err != nil {
		return
	}
	defer f.Close()

	// Decode JSON from file descriptor.
	jd := json.NewDecoder(f)
	for jd.More() {
		if err = jd.Decode(&cfg); err != nil {
			return
		}
	}

	// Truncate and write loaded config with any potential new keys.
	cfg.writeConfig(f)

	return
}

// Get path string of user config dir.
func configDir() (string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	cfgDir := filepath.Join(ucd, "sockchat")

	// Create config dir if necessary. Doesn't break anything existing.
	// If the dir already exists, it will return an fs.ErrExist error.
	// In that case, we just want to continue without returning early.
	if err = os.Mkdir(cfgDir, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}

	return cfgDir, nil
}

// Generate new Config from template.
func newConfig() Config {
	return Config{
		Cookies:  "",
		Host:     "kiwifarms.st",
		Port:     9443,
		Logger:   false,
		ReadOnly: false,
		Room:     1,
		Tor:      false,
		UserID:   -1,

		Proxy: proxyConfig{
			Enabled: false,
			Addr:    "",
			User:    "",
			Pass:    "",
		},
	}
}

// Write loaded config to config.json file.
func (cfg *Config) Save() error {
	return cfg.writeConfig(nil)
}

// f may be provided to reduce the number of file opens but is not required.
func (cfg *Config) writeConfig(f *os.File) error {
	if f == nil {
		f, err := openConfig()
		if err != nil {
			return err
		}
		defer f.Close()
	}

	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	if err = f.Truncate(0); err != nil {
		return err
	}
	if _, err = f.Seek(0, 0); err != nil {
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
