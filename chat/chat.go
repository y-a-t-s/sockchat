package chat

import (
	"context"
	"sync"

	"y-a-t-s/sockchat/config"
)

type Chat struct {
	*sock
	Cfg config.Config

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

	// Split (copy) channel while applying any edits in msg.
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
	// Length tracker.
	// Once we reach the max, the oldest msgs get popped off the feed.
	hl := 0
	// ID of previously processed msg.
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
			// We need to check if the msg was edited and if it's an edit of a msg we already received.
			// Msgs may be edited, but were edited before the client connected.
			// Edits of existing msgs will appear with an older or same ID than the previous msg.
			case msg.IsEdited() && msg.MessageID <= prevID:
				// Split hist into 2 new channels.
				hc, nc := splitChan(msg, hist)

				// Close then send one to the history updates feed.
				close(hc)
				c.History <- hc

				// Point hist to the other copy.
				hist = nc
			default:
				switch {
				case hl < HIST_LEN:
					hl++
				default:
					// Release oldest msg to pool before it's discarded.
					c.sock.pool.Release(<-hist)
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
			case c.ClientUsername == "" && u.ID == uint32(c.Cfg.UserID):
				c.ClientUsername = u.Username
			}

			go c.Users.Add(u)
		}
	}
}
