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
		Messages: make(chan *ChatMessage, cap(s.Messages)),
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
	var hist []*ChatMessage
	prevID := uint32(0)

	appendHist := func(msg *ChatMessage) (appended bool) {
		switch {
		case msg.IsEdited() && msg.MessageID <= prevID:
			hc := make(chan *ChatMessage, len(hist))
			for i := range hist {
				if msg.MessageID == hist[i].MessageID {
					hist[i] = msg
				}
				hc <- hist[i]
			}
			close(hc)
			c.History <- hc
		default:
			hist = append(hist, msg)
			if hl := len(hist); hl > HIST_LEN {
				if hist[0].Author.ID != 0 {
					c.sock.pool.Release(hist[0])
				}
				hist = hist[hl-HIST_LEN:]
			}
			prevID = msg.MessageID
			appended = true
		}

		return appended
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.sock.Messages:
			if msg == nil || !ok {
				continue
			}

			go c.Users.Add(msg.Author)

			if c.logger.feed != nil {
				c.logger.feed <- msg
			}
			if appendHist(msg) {
				c.Messages <- msg
			}
		case m, ok := <-c.Out:
			if !ok {
				continue
			}

			c.sock.out <- m
		case u, ok := <-c.sock.userData:
			if u == nil || !ok {
				continue
			}

			go c.Users.Add(*u)
		}
	}
}
