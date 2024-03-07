package config

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
)

type Config struct {
	Host     string
	Port     uint
	ReadOnly bool
	Room     uint
	UseTor   bool
	Args     []string
}

func ParseArgs() (Config, error) {
	cfg := Config{}

	newEnvError := func(errStr string) error {
		return errors.New(fmt.Sprintf("%s\nCheck .env or specify with argument.", errStr))
	}

	flags := flag.NewFlagSet("SockChat", flag.ContinueOnError)
	flags.StringVar(&cfg.Host, "host", os.Getenv("SC_HOST"), "Specify hostname to connect to.")
	flags.UintVar(&cfg.Port, "port", 0, "Specify outgoing socket port.")
	flags.BoolVar(&cfg.ReadOnly, "ro", false, "Read-only (lurker) mode.")
	flags.UintVar(&cfg.Room, "room", 0, "Room to join by default.")
	flags.BoolVar(&cfg.UseTor, "tor", func() bool {
		if os.Getenv("SC_USE_TOR") == "1" {
			return true
		}

		return false
	}(), "Connect through Tor network.")
	flags.Parse(os.Args[1:])
	// Collect all remaining args that aren't flags. Used for getting cookies.
	cfg.Args = flags.Args()

	if cfg.Host == "" {
		return cfg, newEnvError("Hostname not defined.")
	}

	port := os.Getenv("SC_PORT")
	if port == "" && cfg.Port == 0 {
		return cfg, newEnvError("Port not defined.")
	}
	if cfg.Port == 0 {
		tmp, err := strconv.Atoi(port)
		if err != nil {
			return cfg, err
		}
		cfg.Port = uint(tmp)
	}

	room := os.Getenv("SC_DEF_ROOM")
	if cfg.Room == 0 {
		if room == "" {
			log.Println("Room not defined. Joining General.")
			cfg.Room = 1
		} else {
			tmp, err := strconv.Atoi(room)
			if err != nil {
				return cfg, err
			}
			cfg.Room = uint(tmp)
		}
	}

	return cfg, nil
}
