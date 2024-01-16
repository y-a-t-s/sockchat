package main

import (
	"log"

	"y-a-t-s/sockchat/chat"
)

func main() {
	err := loadEnv()
	if err != nil {
		log.Fatal("Could not process .env", err)
	}

	chat.
		InitChat().
		FetchMessages().
		UI.App.Run()
}
