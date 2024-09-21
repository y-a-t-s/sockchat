package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
)

type jsonData struct {
	// Using json.RawMessage to delay parsing these parts.
	Messages json.RawMessage `json:"messages"`
	Users    json.RawMessage `json:"users"`
}

func (c *Chat) parseResponse(ctx context.Context) {
	parseUser := func(jd *json.Decoder) error {
		u := c.pool.NewUser()
		if err := jd.Decode(&u); err != nil {
			return err
		}

		c.Users.AddUser(u)
		return nil
	}

	parseMsg := func(jd *json.Decoder) (msg *Message, err error) {
		msg = c.pool.NewMsg()

		err = jd.Decode(msg)
		if err != nil {
			return
		}

		msg.MessageRaw = html.UnescapeString(msg.MessageRaw)
		u := c.Users.AddUser(msg.Author)
		msg.Author = u

		return
	}

	parseJson := func(b []byte, out interface{}) error {
		jd := json.NewDecoder(bytes.NewReader(b))

		switch out.(type) {
		case *Message:
			if _, err := jd.Token(); err != nil {
				return err
			}
		}

		var errs []error
		for jd.More() {
			switch out.(type) {
			case *Message:
				msg, err := parseMsg(jd)
				if err != nil {
					errs = append(errs, err)
					continue
				}

				c.sock.messages <- msg
			case *User:
				err := parseUser(jd)
				if err != nil {
					errs = append(errs, err)
				}
			}
		}

		return errors.Join(errs...)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-c.sock.chatJson:
			var jsd jsonData
			if err := json.Unmarshal(m, &jsd); err != nil {
				c.ClientMsg(fmt.Sprintf("Failed to parse server response.\nResponse: %s\n", m), false)
				continue
			}

			if len(jsd.Users) > 0 {
				var u *User
				// Can do async, since order doesn't matter as much here.
				go parseJson(jsd.Users, u)
			}

			if len(jsd.Messages) > 0 {
				var m *Message
				parseJson(jsd.Messages, m)
			}
		}
	}
}
