package chat

import (
	"context"
	"sync"

	"y-a-t-s/sockchat/config"
)

type Chat struct {
	sock *sock
	Cfg  config.Config

	ClientID       int
	ClientUsername string

	Users  *userTable
	logger logger

	History  chan chan *ChatMessage
	Messages chan *ChatMessage
	Out      chan string
}

func NewChat(ctx context.Context, cfg config.Config) (c *Chat, err error) {
	s, err := NewSocket(ctx, cfg)
	if err != nil {
		return
	}

	c = &Chat{
		sock:     s,
		Cfg:      cfg,
		ClientID: cfg.UserID,
		Users:    newUserTable(),
		History:  make(chan chan *ChatMessage, 4),
		Messages: make(chan *ChatMessage, cap(s.messages)),
		Out:      make(chan string, cap(s.out)),
	}

	return
}

func (c *Chat) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	if c.Cfg.Logger {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.logger.Start(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.sock.Start(ctx)
	}()

	c.router(ctx)
	wg.Wait()
}

func (c *Chat) router(ctx context.Context) {
	newHistChan := func() chan *ChatMessage {
		return make(chan *ChatMessage, HIST_LEN)
	}

	splitChan := func(msg *ChatMessage, ch chan *ChatMessage) (chan *ChatMessage, chan *ChatMessage) {
		close(ch)

		a, b := newHistChan(), newHistChan()

		for m := range ch {
			if msg.MessageID == m.MessageID {
				c.sock.pool.Release(m)
				m = msg
			}

			a <- m
			b <- m
		}

		return a, b
	}

	hist := newHistChan()
	hl := 0

	prevID := uint32(0)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.sock.messages:
			if msg == nil || !ok {
				continue
			}

			c.sock.userData <- msg.Author

			if c.logger.feed != nil {
				c.logger.feed <- msg
			}

			switch {
			case msg.IsEdited() && msg.MessageID <= prevID:
				// Split hist into 2 new channels.
				hc, nc := splitChan(msg, hist)
				close(hc)

				c.History <- hc
				hist = nc
			default:
				hl++
				if hl > HIST_LEN {
					c.sock.pool.Release(<-hist)
					hl--
				}

				hist <- msg
				c.Messages <- msg

				prevID = msg.MessageID
			}
		case m, ok := <-c.Out:
			if !ok {
				continue
			}

			c.sock.out <- m
		case u, ok := <-c.sock.userData:
			switch {
			case !ok:
				continue
			case u.ID == uint32(c.Cfg.UserID):
				c.ClientUsername = u.Username
			}

			go c.Users.Add(u)
		}
	}
}
