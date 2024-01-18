package socket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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

type User struct {
	ID        uint32 `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`

	// TODO: Hex colors with random bytes.
	// Color     [3]byte
}

type SocketMessage struct {
	// Using json.RawMessage to delay parsing these parts.
	Messages json.RawMessage `json:"messages"`
	Users    json.RawMessage `json:"users"`
}

func (sock *ChatSocket) GetUsername(id string) *string {
	sock.Users.Mutex.Lock()
	if u, exists := sock.Users.UserMap[id]; exists {
		return &u
	}
	sock.Users.Mutex.Unlock()

	return nil
}

func (sock *ChatSocket) responseHandler() {
	for {
		msg := <-sock.Channels.serverResponse
		if len(msg) == 0 {
			continue
		}

		var sm SocketMessage
		if err := json.Unmarshal(msg, &sm); err != nil {
			sock.ClientMsg(
				fmt.Sprintf("Failed to parse server response.\nResponse: %s\nError: %v",
					string(msg),
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
					sock.ClientMsg(fmt.Sprintf("Failed to parse chat message from server: %v", err))
					continue
				}

				sock.Channels.Messages <- msg
				// Send author data from msg to user handler to prioritize processing relevant users
				sock.Channels.Users <- msg.Author
			}
		}
		if len(sm.Users) != 0 {
			jd := json.NewDecoder(bytes.NewReader(sm.Users))
			for jd.More() {
				var u User
				if err := jd.Decode(&u); err != nil {
					sock.ClientMsg(fmt.Sprintf("Failed to parse user data from server: %v", err))
					continue
				}

				sock.Channels.Users <- u
			}
		}
	}
}

func (u *User) GetUserString() string {
	// Original ANSI spec has 8 basic colors, which the user can set for their terminal theme.
	// 2 of the colors are black and white, which we'll leave out. This leaves 6 remaining colors.
	// We'll exclude the darker blue for better visibility against the standard dark background.
	ansi := []string{"red", "green", "yellow", "magenta", "cyan"}
	color := ansi[u.ID%uint32(len(ansi))]

	// This is kinda ugly, so I'll explain:
	// [%s::u] is a UI style tag for the color and enabling underlining.
	// [-] clears the set color to print the ID (the %d value).
	// [%s] sets the color back to the user's color to print the remaining "):".
	// [-::U] resets the color like before, while also disabling underlining to print the actual message.
	//
	// See https://github.com/rivo/tview/blob/master/doc.go for more info on style tags.
	return fmt.Sprintf("[%s::u]%s ([-]#%d[%s]):[-::U]", color, u.Username, u.ID, color)
}

func (sock *ChatSocket) ClientMsg(msg string) {
	sock.Channels.Messages <- ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageRaw: msg,
	}
}
