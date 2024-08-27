package config

import (
	"errors"
	"flag"
	"os"
)

func (cfg *Config) ParseArgs() error {
	flags := flag.NewFlagSet("SockChat", flag.ContinueOnError)
	flags.StringVar(&cfg.Cookies, "cookies", cfg.Cookies, "Set cookies used to connect.")
	flags.StringVar(&cfg.Host, "host", cfg.Host, "Specify hostname to connect to.")
	flags.BoolVar(&cfg.Logger, "log", cfg.Logger, "Enable chat logger.")
	flags.UintVar(&cfg.Port, "port", cfg.Port, "Specify outgoing socket port.")
	flags.UintVar(&cfg.Room, "room", cfg.Room, "Room to join by default.")
	flags.BoolVar(&cfg.Tor, "tor", cfg.Tor, "Connect through Tor network.")
	flags.BoolVar(&cfg.ReadOnly, "ro", cfg.ReadOnly, "Read-only (lurker) mode.")
	// flags.BoolVar(&cfg.ApiMode, "api", cfg.ApiMode, "Start in API mode. See the documentation.")
	flags.Parse(os.Args[1:])

	switch {
	case cfg.Cookies == "":
		return errors.New("No cookies provided. Set them in the config file.")
	case cfg.Host == "":
		return errors.New("Hostname not defined.")
	}

	return nil
}
