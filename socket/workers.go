package socket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"github.com/gorilla/websocket"
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

func (s *sock) msgReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if s.Conn == nil {
			s.connect(ctx)
		}

		_, msg, err := s.ReadMessage()
		if err != nil {
			s.ClientMsg("Failed to read from socket.\n")
			s.connect(ctx)
		}

		s.incoming <- msg
	}
}

func (s *sock) msgWriter(ctx context.Context) {
	joinRE := regexp.MustCompile(`^/join (\d)+$`)
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.outgoing:
			// Trim unnecessary whitespace.
			msg = bytes.TrimSpace(msg)
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
				s.room = uint(tmp)
			}

			if err := s.write(msg); err != nil && !errors.Is(err, websocket.ErrCloseSent) {
				// Send it back to the queue to try again.
				s.outgoing <- msg
			}
		}
	}
}

func (s *sock) startWorkers(ctx context.Context) error {
	go s.msgReader(ctx)
	go s.userHandler(ctx)
	if !s.readOnly {
		go s.msgWriter(ctx)
	}
	return s.responseHandler(ctx)
}

func (s *sock) responseHandler(ctx context.Context) error {
	// out has to be passed as a pointer for the json Decode to work.
	parseResponse := func(b []byte, out interface{}) error {
		jd := json.NewDecoder(bytes.NewReader(b))

		switch out.(type) {
		case *ChatMessage:
			if _, err := jd.Token(); err != nil {
				return err
			}
		}

		errs := []error{nil}
		for jd.More() {
			if err := jd.Decode(out); err != nil {
				log.Printf("Failed to parse data from server.\nError: %v", err)
				errs = append(errs, err)
				continue
			}

			switch out.(type) {
			case *ChatMessage:
				msg := *(out.(*ChatMessage))
				s.messages <- msg
				s.users <- msg.Author
			case *User:
				s.users <- *(out.(*User))
			}
		}

		return errors.Join(errs...)
	}

	process := func(msg []byte) error {
		if len(msg) == 0 {
			return errors.New("Empty message from server.")
		}

		// Server sometimes sends plaintext messages to client.
		// This typically happens when it sends error messages.
		if !json.Valid(msg) {
			s.ClientMsg(string(msg))
		}

		var sm serverResponse
		if err := json.Unmarshal(msg, &sm); err != nil {
			s.ClientMsg(fmt.Sprintf("Failed to parse server response.\nResponse: %s\n", msg))
			return err
		}

		if len(sm.Messages) > 0 {
			parseResponse(sm.Messages, &ChatMessage{})
		}
		if len(sm.Users) > 0 {
			parseResponse(sm.Users, &User{})
		}

		return nil
	}

	errs := []error{nil}
	for {
		select {
		case <-ctx.Done():
			return errors.Join(errs...)
		case msg := <-s.incoming:
			if err := process(msg); err != nil {
				errs = append(errs, err)
			}
		}
	}
}
