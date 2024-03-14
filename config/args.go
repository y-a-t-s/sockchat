package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func (cfg *Config) ParseArgs() error {
	newEnvError := func(errStr string) error {
		return errors.New(fmt.Sprintf("%s\nCheck config or specify with argument.", errStr))
	}

	flags := flag.NewFlagSet("SockChat", flag.ContinueOnError)
	flags.StringVar(&cfg.Host, "host", cfg.Host, "Specify hostname to connect to.")
	flags.BoolVar(&cfg.Logger, "log", cfg.Logger, "Enable chat logger.")
	flags.UintVar(&cfg.Port, "port", cfg.Port, "Specify outgoing socket port.")
	flags.BoolVar(&cfg.ReadOnly, "ro", cfg.ReadOnly, "Read-only (lurker) mode.")
	flags.UintVar(&cfg.Room, "room", cfg.Room, "Room to join by default.")
	flags.BoolVar(&cfg.Tor, "tor", cfg.Tor, "Connect through Tor network.")
	flags.Parse(os.Args[1:])
	// Collect all remaining args that aren't flags. Used for getting cookies.
	cfg.Args = flags.Args()

	if cfg.Host == "" {
		return newEnvError("Hostname not defined.")
	}

	return nil
}
