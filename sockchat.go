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
	defer sock.Conn.CloseNow()

	ui := chatui.NewUI(sock)
	if ui == nil {
		log.Fatal("Could not init UI.")
	}

	go ui.ChatHandler(sock)
	go sock.Fetch()

	log.Fatal(ui.App.Run())
}
