package main

import (
	"log"

	"y-a-t-s/sockchat/chat"
	"y-a-t-s/sockchat/tui"
)

func main() {
	err := loadEnv()
	if err != nil {
		log.Fatal("Could not process .env", err)
	}

	log.Println("Opening socket...")
	sock := chat.NewSocket().Connect()

	ui := tui.NewUI(sock)
	if ui == nil {
		log.Fatal("Could not init UI.")
	}

	go ui.ChatHandler(sock, nil)
	go sock.Fetch()

	defer sock.Conn.CloseNow()
	ui.App.Run()
}
