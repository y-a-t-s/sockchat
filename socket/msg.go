package socket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type ChatMessage struct {
	Author          User   `json:"author"`
	Message         string `json:"message"`
	MessageRaw      string `json:"message_raw"`
	MessageID       uint32 `json:"message_id"`
	MessageDate     uint64 `json:"message_date"`
	MessageEditDate uint64 `json:"message_edit_date"`
	RoomID          uint16 `json:"room_id"`
}

type SocketMessage struct {
	// Using json.RawMessage to delay parsing these parts.
	Messages json.RawMessage `json:"messages"`
	Users    json.RawMessage `json:"users"`
}

func (sock *ChatSocket) ChatDebug(msg string) bool {
	if !strings.HasPrefix(msg, "!debug") {
		return false
	}

	cmd := strings.SplitN(msg, " ", 3)
	if len(cmd) < 2 {
		return true
	}

	switch cmd[1] {
	case "clientMsg":
		if len(cmd) == 3 {
			sock.ClientMsg(cmd[2])
		}
	}

	return true
}

func (sock *ChatSocket) ClientMsg(msg string) {
	cm := ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageRaw: msg,
	}

	sock.Channels.Messages <- cm
}

func (sock *ChatSocket) responseHandler() {
	defer sock.Conn.Close()

	for {
		msg := <-sock.Channels.serverResponse
		if len(msg) == 0 {
			continue
		}

		// Error messages from the server usually aren't encoded.
		if !json.Valid(msg) {
			sock.ClientMsg(string(msg))
			continue
		}

		var sm SocketMessage
		if err := json.Unmarshal(msg, &sm); err != nil {
			sock.ClientMsg(
				fmt.Sprintf("Failed to parse server response.\nResponse: %s\nError: %v",
					msg,
					err))
		}

		if len(sm.Messages) != 0 {
			jd := json.NewDecoder(bytes.NewReader(sm.Messages))
			if _, err := jd.Token(); err != nil {
				log.Fatal(err)
			}

			for jd.More() {
				var msg ChatMessage
				if err := jd.Decode(&msg); err != nil {
					sock.ClientMsg(
						fmt.Sprintf("Failed to parse message from server.\nError: %v",
							err))
				} else {
					sock.Channels.Messages <- msg
					// Send user data from msg to user handler to prioritize active users
					sock.Channels.Users <- msg.Author
				}
			}
		}
		if len(sm.Users) != 0 {
			jd := json.NewDecoder(bytes.NewReader(sm.Users))
			for jd.More() {
				var u User
				if err := jd.Decode(&u); err != nil {
					sock.ClientMsg(
						fmt.Sprintf("Failed to parse user data from server: %v",
							err))
				} else {
					sock.Channels.Users <- u
				}
			}
		}
	}
}
