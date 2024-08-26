package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
)

type serverResp struct {
	// Using json.RawMessage to delay parsing these parts.
	Messages json.RawMessage `json:"messages"`
	Users    json.RawMessage `json:"users"`
}

func (c *Chat) parseResponse(msg []byte) error {
	if msg == nil || len(msg) == 0 {
		return errors.New("Received empty message from server.")
	}

	parseJson := func(b []byte, out interface{}) error {
		jd := json.NewDecoder(bytes.NewReader(b))

		switch out.(type) {
		case *ChatMessage:
			if _, err := jd.Token(); err != nil {
				return err
			}
		}

		var errs []error
		for jd.More() {
			switch out.(type) {
			case *ChatMessage:
				msg := c.pool.NewMsg()

				if err := jd.Decode(msg); err != nil {
					errs = append(errs, err)
					continue
				}
				msg.MessageRaw = html.UnescapeString(msg.MessageRaw)

				if qu := c.Users.QueryUser(msg.Author.ID); qu != nil {
					c.pool.Release(msg.Author)
					msg.Author = qu
				} else {
					go c.Users.Add(msg.Author)
				}

				c.sock.messages <- msg
			case *User:
				u := c.pool.NewUser()
				if err := jd.Decode(&u); err != nil {
					errs = append(errs, err)
					continue
				}

				go func() {
					if !c.Users.Add(u) {
						c.pool.Release(u)
					}
				}()
			}
		}

		return errors.Join(errs...)
	}

	// Server sometimes sends plaintext messages to client.
	// This typically happens when it sends error messages.
	if !json.Valid(msg) {
		return errors.New(string(msg))
	}

	var sm serverResp
	if err := json.Unmarshal(msg, &sm); err != nil {
		return errors.New(fmt.Sprintf("Failed to parse server response.\nResponse: %s\n", msg))
	}

	if len(sm.Users) > 0 {
		var u *User
		// Can do async, since order doesn't matter as much here.
		go parseJson(sm.Users, u)
	}

	if len(sm.Messages) > 0 {
		var m *ChatMessage
		parseJson(sm.Messages, m)
	}

	return nil
}
