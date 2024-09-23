package chat

import (
	"context"
	"fmt"
	"strings"

	"y-a-t-s/sockchat/config"

	"github.com/gen2brain/beeep"
)

type Chat struct {
	*sock

	Cfg config.Config

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
		Users: &userTable{
			ClientID: uint32(cfg.UserID),
		},
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

func (c *Chat) recordHistory(feed <-chan *Message) chan chan *Message {
	out := make(chan chan *Message, 4)

	var prevID uint32 = 0 // ID of previously processed msg.

	hist := newFeedChan()
	editHist := func(msg *Message) {
		close(hist)

		hc, nc := newFeedChan(), newFeedChan()
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
				select {
				case hist <- msg:
				default:
					hm := <-hist
					hm.Release()
					hist <- msg
				}
				prevID = msg.MessageID
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
		logFeed := c.Feeder.Feed()
		context.AfterFunc(ctx, func() {
			logFeed.Close()
		})

		err := startLogger(logFeed.Feed)
		if err != nil {
			panic(err)
		}

	}

	msgHandler := func(msg *Message) {
		if msg == nil {
			return
		}

		if c.Users.ClientName != "" && strings.Contains(msg.MessageRaw, fmt.Sprintf("@%s,", c.Users.ClientName)) {
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
