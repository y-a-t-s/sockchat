package main

import (
	"github.com/joho/godotenv"
	"log"
)

func newEnv() {
	envMap := make(map[string]string)
	// Ideally .net should be here, but the redirect doesn't always work.
	envMap["SC_HOST"] = "kiwifarms.hk"
	envMap["SC_PORT"] = "9443"

	godotenv.Write(envMap, ".env")
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Print(".env file not found. Creating new one.")
		newEnv()
	}
}
