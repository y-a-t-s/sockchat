package chat

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sync"

	"y-a-t-s/sockchat/config"

	"github.com/y-a-t-s/libkiwi"
)

type Chat struct {
	*sock
	Cfg config.Config
	kf  *libkiwi.KF

	ClientID       int
	ClientUsername string

	Users  *userTable
	pool   ChatPool
	logger logger

	History  chan chan *ChatMessage
	Messages chan *ChatMessage
}

func NewChat(ctx context.Context, cfg config.Config) (c *Chat, err error) {
	s, err := newSocket(ctx, cfg)
	if err != nil {
		return
	}

	hc := http.Client{}
	if s.proxy != nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DialContext = s.proxy.DialContext
		hc.Transport = tr
	}

	kf, err := libkiwi.NewKF(hc, cfg.Host, cfg.Args[0])
	if err != nil {
		return
	}

	c = &Chat{
		sock:     s,
		Cfg:      cfg,
		kf:       kf,
		ClientID: cfg.UserID,
		Users:    newUserTable(),
		pool:     newChatPool(),
		History:  make(chan chan *ChatMessage, 4),
		Messages: make(chan *ChatMessage, cap(s.messages)),
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

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		stopf := context.AfterFunc(ctx, func() {
			c.sock.Stop()
		})
		defer stopf()

		c.msgReader(ctx)
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
				c.pool.Release(m)
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

	joinRE := regexp.MustCompile(`^/join \d+`)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.sock.messages:
			if msg == nil || !ok {
				continue
			}

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
					c.pool.Release(<-hist)
				}

				hist <- msg
				c.Messages <- msg

				prevID = msg.MessageID
			}
		case m, ok := <-c.Out:
			switch {
			case !ok, c.Cfg.ReadOnly && !joinRE.MatchString(m):
				continue
			default:
				err := c.sock.write(m)
				if err != nil {
					c.ClientMsg(fmt.Sprintf("Failed to send: %s\nError: %s\n", m, err))
				}
			}
		}
	}
}
