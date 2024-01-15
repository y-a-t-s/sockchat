package main

import (
	"log"

	"y-a-t-s/sockchat/chatui"
	"y-a-t-s/sockchat/socket"
)

func main() {
	loadEnv()

	log.Println("Opening socket...")
	sock := socket.NewSocket().Connect()

	ui := chatui.NewUI(sock)
	if ui == nil {
		log.Fatal("Could not init UI.")
	}

	go ui.ChatHandler(sock, nil)
	go sock.Fetch()

	defer sock.Conn.CloseNow()
	ui.App.Run()
}
