package socket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
)

type ChatMessage struct {
	Author          User   `json:"author"`
	Message         string `json:"message"`
	MessageRaw      string `json:"message_raw"`
	MessageID       uint32 `json:"message_id"`
	MessageDate     int64  `json:"message_date"`
	MessageEditDate int64  `json:"message_edit_date"`
	RoomID          uint16 `json:"room_id"`
}

type serverResponse struct {
	// Using json.RawMessage to delay parsing these parts.
	Messages json.RawMessage `json:"messages"`
	Users    json.RawMessage `json:"users"`
}

func (sock *Socket) fetch() {
	for {
		if sock.Conn == nil {
			sock.connect()
		}

		_, msg, err := sock.ReadMessage()
		if err != nil {
			sock.ClientMsg("Failed to read from socket.\n")
			sock.connect()
		}

		sock.incoming <- msg
	}
}

func (sock *Socket) msgWriter() {
	joinRE := regexp.MustCompile(`^/join (\d)+$`)
	for {
		// Trim unnecessary whitespace.
		msg := bytes.TrimSpace(<-sock.Outgoing)
		// Ignore empty messages.
		if len(msg) == 0 {
			continue
		}

		// Update selected room if /join message was sent.
		if room := joinRE.FindSubmatch(msg); room != nil {
			tmp, err := strconv.Atoi(string(room[1]))
			if err != nil {
				continue
			}
			sock.room = uint(tmp)
		}

		sock.write(msg)
	}
}

func (sock *Socket) responseHandler() {
	go sock.fetch()
	go sock.userHandler()
	if !sock.readOnly {
		go sock.msgWriter()
	}

	// out has to be passed as a pointer for the json Decode to work.
	parseResponse := func(b []byte, out interface{}) {
		jd := json.NewDecoder(bytes.NewReader(b))

		switch out.(type) {
		case *ChatMessage:
			if _, err := jd.Token(); err != nil {
				log.Fatal(err)
			}
		}

		for jd.More() {
			if err := jd.Decode(out); err != nil {
				log.Printf("Failed to parse data from server.\nError: %v", err)
				continue
			}

			switch out.(type) {
			case *ChatMessage:
				msg := *(out.(*ChatMessage))
				sock.Messages <- msg
				sock.users <- msg.Author
			case *User:
				sock.users <- *(out.(*User))
			}
		}
	}

	for {
		msg := <-sock.incoming
		if len(msg) == 0 {
			continue
		}

		// Error messages from the server usually aren't encoded.
		if !json.Valid(msg) {
			sock.ClientMsg(string(msg))
			continue
		}

		var sm serverResponse
		if err := json.Unmarshal(msg, &sm); err != nil {
			sock.ClientMsg(
				fmt.Sprintf("Failed to parse server response.\nResponse: %s\nError: %v",
					msg,
					err))
			continue
		}

		if len(sm.Messages) > 0 {
			parseResponse(sm.Messages, &ChatMessage{})
		}
		if len(sm.Users) > 0 {
			parseResponse(sm.Users, &User{})
		}
	}
}
