package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func newEnv(envMap map[string]string) {
	godotenv.Write(envMap, ".env")
	loadEnv()
}

func checkEnv(envMap map[string]string) error {
	f, err := os.Open(".env")
	if err != nil {
		return err
	}
	defer f.Close()

	for k, v := range envMap {
		_, exists := os.LookupEnv(k)
		if !exists {
			log.Printf("Adding %s to .env", k)
			fmt.Fprintf(f, "%s=\"%s\"", k, v)
		}
	}

	return nil
}

func loadEnv() error {
	envMap := make(map[string]string)
	envMap["SC_DEF_ROOM"] = "1"
	// Ideally .net should be here, but the redirect doesn't always work.
	envMap["SC_HOST"] = "kiwifarms.hk"
	envMap["SC_PORT"] = "9443"
	envMap["SC_USER_ID"] = ""

	err := godotenv.Load()
	if err != nil {
		log.Print(".env file not found. Creating new one.")
		newEnv(envMap)
	}

	// Check for missing values and return errors
	return checkEnv(envMap)
}
