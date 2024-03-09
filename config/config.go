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
	var cfg Config

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return cfg, err
	}

	fname := fmt.Sprintf("%s/sockchat/config.json", cfgDir)
	// Open .env file for RW.
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg = newConfig()
			err = writeConfig(cfg)
		}
		return cfg, err
	}
	defer f.Close()

	jd := json.NewDecoder(f)
	for jd.More() {
		if err := jd.Decode(&cfg); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
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

func writeConfig(cfg Config) error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.Mkdir(fmt.Sprintf("%s/sockchat", cfgDir), 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	fname := fmt.Sprintf("%s/sockchat/config.json", cfgDir)
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return err
	}

	if _, err := f.Write(b); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	return nil
}
