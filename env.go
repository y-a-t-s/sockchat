package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func newEnv(envMap map[string]string) error {
	godotenv.Write(envMap, ".env")
	return loadEnv()
}

func checkEnv(envMap map[string]string) error {
	// Open .env file for RW.
	f, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	efMap, err := godotenv.Parse(f)
	if err != nil {
		return err
	}

	//
	for k, v := range envMap {
		_, exists := efMap[k]
		if !exists {
			log.Printf("Adding %s to .env", k)
			fmt.Fprintf(f, "%s=\"%s\"", k, v)
		}
	}

	return nil
}

func loadEnv() error {
	envMap := map[string]string{
		"SC_DEF_ROOM": "1",
		"SC_USER_ID":  "",

		"SC_HOST": "kiwifarms.net",
		"SC_PORT": "9443",
		// If SC_HOST is a .onion domain, this is ignored.
		// Useful for clearnet-over-tor.
		"SC_USE_TOR": "0",
	}

	// Check for missing values.
	if err := checkEnv(envMap); err != nil {
		return err
	}

	if err := godotenv.Load(); err != nil {
		log.Println(".env file not found. Creating new one.")
		return newEnv(envMap)
	}

	return nil
}
