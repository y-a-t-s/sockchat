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
	UserID   int    `json:"user_id"`

	Proxy proxyConfig `json:"proxy"`
	Tor   torConfig   `json:"tor"`

	// ApiMode bool `json:",omitempty"`

	// Used for collecting remaining args, containing cookies.
	Args []string `json:",omitempty"`
}

type torConfig struct {
	Enabled  bool   `json:"enabled"`
	Onion    string `json:"onion_host"`
	Clearnet bool   `json:"clearnet_over_tor"`
}

func newTorConfig() torConfig {
	return torConfig{
		Enabled:  false,
		Onion:    "kiwifarmsaaf4t2h7gc3dfc5ojhmqruw2nit3uejrpiagrxeuxiyxcyd.onion",
		Clearnet: false,
	}
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
		UserID:   -1,

		Proxy: proxyConfig{
			Enabled: false,
			Addr:    "",
			User:    "",
			Pass:    "",
		},
		Tor: newTorConfig(),
	}
}

func (cfg *Config) UnmarshalJSON(b []byte) error {
	var cm map[string]interface{}

	parseProxyCfg := func(m map[string]interface{}) {
		for k, v := range m {
			switch k {
			case "enabled":
				cfg.Proxy.Enabled = v.(bool)
			case "address":
				cfg.Proxy.Addr = v.(string)
			case "username":
				cfg.Proxy.User = v.(string)
			case "password":
				cfg.Proxy.Pass = v.(string)
			}
		}
	}

	parseTorCfg := func(m map[string]interface{}) {
		for k, v := range m {
			switch k {
			case "enabled":
				cfg.Tor.Enabled = v.(bool)
			case "onion_host":
				cfg.Tor.Onion = v.(string)
			case "clearnet_over_tor":
				cfg.Tor.Clearnet = v.(bool)
			}
		}
	}

	err := json.Unmarshal(b, &cm)
	if err != nil {
		return err
	}

	for k, v := range cm {
		switch k {
		case "cookies":
			cfg.Cookies = v.(string)
		case "host":
			cfg.Host = v.(string)
		case "port":
			cfg.Port = uint(v.(float64))
		case "logger":
			cfg.Logger = v.(bool)
		case "read_only":
			cfg.ReadOnly = v.(bool)
		case "room":
			cfg.Room = uint(v.(float64))
		case "user_id":
			cfg.UserID = int(v.(float64))
		case "proxy":
			parseProxyCfg(v.(map[string]interface{}))
		case "tor":
			switch v.(type) {
			// Migrate deprecated config value.
			// Will eventually be removed in future versions.
			case bool:
				cfg.Tor = newTorConfig()
				cfg.Tor.Enabled = v.(bool)
			case map[string]interface{}:
				parseTorCfg(v.(map[string]interface{}))
			}
		}
	}

	return nil
}

// Write loaded config to config.json file.
func (cfg *Config) Save() error {
	return cfg.writeConfig(nil)
}

// f may be provided to reduce the number of file opens but is not required.
func (cfg *Config) writeConfig(f *os.File) (err error) {
	if f == nil {
		f, err = openConfig()
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
