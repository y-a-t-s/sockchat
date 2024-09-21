package chat

import (
	"context"
	"fmt"
	"strings"

	"y-a-t-s/sockchat/config"

	"github.com/gen2brain/beeep"
)

type client struct {
	ID       int
	Username string
}

type Chat struct {
	*sock

	Cfg    config.Config
	Client *client

	Users *userTable
	pool  *ChatPool

	History  chan chan *Message
	Messages chan *Message

	Feeder *Feeder
}

func NewChat(ctx context.Context, cfg config.Config) (*Chat, error) {
	s, err := newSocket(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &Chat{
		sock: s,
		Cfg:  cfg,
		Client: &client{
			ID: cfg.UserID,
		},
		Users:    &userTable{},
		pool:     newChatPool(),
		History:  make(chan chan *Message, 4),
		Messages: make(chan *Message, cap(s.messages)),
		Feeder:   NewFeeder(ctx),
	}, nil
}

func (c *Chat) Start(ctx context.Context) {
	go c.parseResponse(ctx)
	go c.sock.Start(ctx)
	c.router(ctx)

	c.Stop()
}

func (c *Chat) Stop() {
	// Save socket's config state.
	// Used to overwrite saved cookie string on exit.
	c.Cfg = c.sock.cfg
}

func newHistChan() chan *Message {
	return make(chan *Message, HIST_LEN)
}

func (c *Chat) recordHistory(feed <-chan *Message) chan chan *Message {
	out := make(chan chan *Message, 2)

	var (
		prevID uint32 = 0 // ID of previously processed msg.

		// Length tracker.
		// Once we reach the max, the oldest msgs get popped off the feed.
		hl   = 0
		hist = newHistChan()
	)

	editHist := func(msg *Message) {
		close(hist)

		hc, nc := newHistChan(), newHistChan()
		for hm := range hist {
			if msg.MessageID == hm.MessageID {
				hm.Release()
				hm = msg
			}

			hc <- hm
			nc <- hm
		}

		close(hc)
		out <- hc
		hist = nc
	}

	go func() {
		defer close(out)

		for msg := range feed {
			switch {
			// We need to check if the msg was edited and if it's an edit of a msg we already received.
			// Msgs may be edited, but were edited before the client connected.
			// Edits of existing msgs will appear with an older or same ID than the previous msg.
			case msg.IsEdited() && msg.MessageID <= prevID:
				editHist(msg)
			default:
				switch {
				case hl >= HIST_LEN:
					hm := <-hist
					hm.Release()
					hist <- msg
				default:
					hl++
				}
			}
		}
	}()

	return out
}

func (c *Chat) router(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	histFeed := make(chan *Message, HIST_LEN)
	defer close(histFeed)

	hist := c.recordHistory(histFeed)

	if c.Cfg.Logger {
		logFeed := c.Feeder.NewFeed()
		defer logFeed.Close()

		err := startLogger(logFeed.Feed)
		if err != nil {
			panic(err)
		}

		stopf := context.AfterFunc(ctx, func() {
			logFeed.Close()
		})
		defer stopf()
	}

	msgHandler := func(msg *Message) {
		if msg == nil {
			return
		}

		if c.Client.Username != "" && strings.Contains(msg.MessageRaw, fmt.Sprintf("@%s,", c.Client.Username)) {
			msg.IsMention = true
			beeep.Notify("New mention", msg.MessageRaw, "")
		}

		c.Feeder.Send(msg)
		histFeed <- msg
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
				// case e, ok := <-c.sock.errLog:
				// if !ok {
				// return
				// }
			case dms, ok := <-c.sock.debug:
				if !ok {
					return
				}
				c.ClientMsg(dms, true)
			case ms, ok := <-c.sock.infoLog:
				if !ok {
					return
				}
				c.ClientMsg(ms, false)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		case hf := <-hist:
			c.History <- hf
		case msg := <-c.sock.messages:
			if msg != nil {
				msgHandler(msg)
			}
		}
	}
}
