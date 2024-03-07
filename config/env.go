package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type envMap map[string]string

func newEnvMap() envMap {
	return map[string]string{
		"SC_DEF_ROOM": "1",
		"SC_USER_ID":  "",

		"SC_HOST": "kiwifarms.net",
		"SC_PORT": "9443",
		// If SC_HOST is a .onion domain, this is ignored.
		// Useful for clearnet-over-tor.
		"SC_USE_TOR": "0",
	}
}

func (em envMap) writeEnv() error {
	godotenv.Write(em, ".env")
	return LoadEnv()
}

func (em envMap) checkEnv() error {
	// Open .env file for RW.
	f, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fMap, err := godotenv.Parse(f)
	if err != nil {
		return err
	}

	for k, v := range em {
		_, exists := fMap[k]
		if !exists {
			log.Printf("Adding %s to .env", k)
			fmt.Fprintf(f, "%s=\"%s\"", k, v)
		}
	}

	return nil
}

// TODO: turn envMap into type and use receiver methods.
func LoadEnv() error {
	em := newEnvMap()

	// Check for missing values.
	if err := em.checkEnv(); err != nil {
		return err
	}

	if err := godotenv.Load(); err != nil {
		log.Println(".env file not found. Creating new one.")
		return em.writeEnv()
	}

	return nil
}
