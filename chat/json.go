package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"sync"
)

type ErrUnexpectedToken struct {
	token json.Token
}

func (e *ErrUnexpectedToken) Error() string {
	return fmt.Sprintf("Received unexpected JSON token when decoding: %s (%T)", e.token, e.token)
}

type ErrBadResponse struct {
	content []byte
}

func (e *ErrBadResponse) Error() string {
	return fmt.Sprintf("Failed to parse response from server: %s", e.content)
}

type ServerResponse struct {
	Messages []json.RawMessage `json:"messages"` // Array of chat messages.
	Users    json.RawMessage   `json:"users"`    // Obj containing user records.
}

func (s *sock) ParseResponse(ctx context.Context, b []byte) (ServerResponse, error) {
	var sr ServerResponse
	err := json.Unmarshal(b, &sr)
	if err != nil {
		return sr, err
	}

	go func() {
		msgs, errs := s.ParseMessages(ctx, sr)
		go func() {
			for err := range errs {
				s.errLog <- err
			}
		}()
		for msg := range msgs {
			s.messages <- msg
		}
	}()

	go func() {
		users, errs := s.ParseUserRecords(ctx, sr)
		go func() {
			for err := range errs {
				s.errLog <- err
			}
		}()
		for user := range users {
			s.Users.AddUser(user)
			user.Release()
		}
	}()

	return sr, nil
}

func (s *sock) ParseMessage(jd *json.Decoder) (*Message, error) {
	msg := s.pool.NewMsg()

	err := jd.Decode(msg)
	if err != nil {
		return nil, err
	}

	msg.Message = html.UnescapeString(msg.Message)
	msg.MessageRaw = html.UnescapeString(msg.MessageRaw)

	return msg, nil
}

func (s *sock) ParseUser(jd *json.Decoder) (*User, error) {
	u := s.pool.NewUser()

	err := jd.Decode(u)
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (s *sock) ParseUserRecords(ctx context.Context, sr ServerResponse) (<-chan *User, <-chan error) {
	if len(sr.Users) == 0 {
		return nil, nil
	}

	out := make(chan *User, 512)
	errs := make(chan error, 1)
	closeAll := sync.OnceFunc(func() {
		close(out)
		close(errs)
	})

	go func() {
		defer closeAll()

		r := bytes.NewReader(sr.Users)
		jd := json.NewDecoder(r)
		// Attempt to read opening delim.
		_, err := jd.Token()
		if err != nil {
			errs <- err
			return
		}

		for jd.More() {
			t, err := jd.Token()
			if err != nil {
				if err == io.EOF {
					break
				}

				errs <- err
				return
			}

			// Expected layout is: {"160024": {"id": "160024", "username": "y a t s", ...}, ...}
			// We want to skip past the outer ID strings since they can't be predictably tagged.
			switch t.(type) {
			case string:
				// Parse user data that occurs after current token.
				// Slightly counterintuitive by appearance.
				u, err := s.ParseUser(jd)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case errs <- err:
						continue
					}
				}

				select {
				case <-ctx.Done():
					return
				case out <- u:
				}
			default:
				errs <- &ErrUnexpectedToken{t}
			}
		}
	}()

	return out, errs
}

func jsArrayToReader(ja []json.RawMessage) (io.Reader, error) {
	bb := &bytes.Buffer{}

	// Write elements as JSONLines to buffer, suitable for use in a decoder.
	for _, elem := range ja {
		_, err := bb.Write(elem)
		if err != nil {
			return nil, err
		}
		err = bb.WriteByte('\n')
		if err != nil {
			return nil, err
		}
	}

	return bb, nil
}

func (s *sock) ParseMessages(ctx context.Context, sr ServerResponse) (<-chan *Message, <-chan error) {
	out := make(chan *Message, 128)
	errs := make(chan error, 1)
	closeAll := sync.OnceFunc(func() {
		close(out)
		close(errs)
	})

	go func() {
		defer closeAll()

		r, err := jsArrayToReader(sr.Messages)
		if err != nil {
			errs <- err
			return
		}

		jd := json.NewDecoder(r)
		for jd.More() {
			msg, err := s.ParseMessage(jd)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case errs <- err:
				}

				continue
			}

			u := s.Users.AddUser(msg.Author)
			msg.Author.Release()
			msg.Author = u

			select {
			case <-ctx.Done():
				return
			case out <- msg:
			}
		}
	}()

	return out, errs
}

// func (s *sock) parseUserTable(ctx context.Context, b []byte) (<-chan *User, <-chan error) {
// return s.parseUserTable(ctx, sr.Users)
// }
