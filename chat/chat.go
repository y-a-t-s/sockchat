package chat

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

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
	pool  ChatPool

	logger logger

	History  chan chan *Message
	Messages chan *Message

	feeds []chan *Message
	mutex *sync.Mutex
}

func NewChat(ctx context.Context) (c *Chat, err error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return
	}
	err = cfg.ParseArgs()
	if err != nil {
		return
	}

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

	l, err := newLogger(cfg.Logger)
	if err != nil {
		return
	}

	c = &Chat{
		sock: s,
		Cfg:  cfg,
		Client: &client{
			ID: cfg.UserID,
		},
		Users:    newUserTable(),
		pool:     newChatPool(),
		logger:   l,
		History:  make(chan chan *Message, 4),
		Messages: make(chan *Message, cap(s.messages)),
		mutex:    &sync.Mutex{},
	}

	context.AfterFunc(ctx, func() {
		close(c.Users.in)
	})

	return
}

func (c *Chat) NewFeed() chan *Message {
	cm := make(chan *Message, 2048)

	go func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		c.feeds = append(c.feeds, cm)
	}()

	return cm
}

func (c *Chat) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.logger.Start(ctx)
	}()

	go c.parseResponse(ctx)
	go c.sock.Start(ctx)
	c.router(ctx)

	cancel()
	c.Stop()
	wg.Wait()
}

func (c *Chat) saveCookieState() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Cookies = c.sock.cookies[0]

	err = cfg.Save()
	if err != nil {
		return err
	}

	return nil
}

func (c *Chat) Stop() error {
	err := c.saveCookieState()
	if err != nil {
		return err
	}

	c.sock.Stop()

	return nil
}

func (c *Chat) router(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	newHistChan := func() chan *Message {
		return make(chan *Message, HIST_LEN)
	}

	// Length tracker.
	// Once we reach the max, the oldest msgs get popped off the feed.
	hl := 0
	hist := newHistChan()

	// ID of previously processed msg.
	prevID := uint32(0)

	appendHist := func(msg *Message) {
		switch {
		case hl >= HIST_LEN:
			c.pool.Release(<-hist)
		default:
			hl++
		}
		hist <- msg
	}

	editHandler := func(msg *Message) {
		close(hist)

		hc, nc := newHistChan(), newHistChan()
		for m := range hist {
			if msg.MessageID == m.MessageID {
				c.pool.Release(m)
				m = msg
			}
			hc <- m
			nc <- m
		}

		// Close then send one to the history updates feed.
		close(hc)
		c.History <- hc
		hist = nc
	}

	msgHandler := func(msg *Message) {
		if msg == nil {
			return
		}

		if c.logger.feed != nil {
			c.logger.feed <- msg
		}

		if c.Client.Username != "" && strings.Contains(msg.MessageRaw, fmt.Sprintf("@%s,", c.Client.Username)) {
			msg.IsMention = true
			beeep.Notify("New mention", msg.MessageRaw, "")
		}

		switch {
		// We need to check if the msg was edited and if it's an edit of a msg we already received.
		// Msgs may be edited, but were edited before the client connected.
		// Edits of existing msgs will appear with an older or same ID than the previous msg.
		case msg.IsEdited() && msg.MessageID <= prevID:
			editHandler(msg)
		default:
			appendHist(msg)

			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			go func() {
				defer cancel()

				c.mutex.Lock()
				defer c.mutex.Unlock()

				for _, feed := range c.feeds {
					if feed != nil {
						feed <- msg
					}
				}

				prevID = msg.MessageID
			}()

			<-ctx.Done()
		}
	}

	// Split off info-related stuff like user data due to locking.
	go func() {
		for {
			select {
			case <-ctx.Done():
				cancel()
				return
			case e := <-c.sock.errLog:
				c.logger.errs.Error(e.Error())
			case u := <-c.sock.userData:
				if u == nil {
					continue
				}

				if c.Client.Username == "" && u.ID == uint32(c.Cfg.UserID) {
					c.Client.Username = u.Username
				}

				if !c.Users.Add(u) {
					c.pool.Release(u)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return
		case dms := <-c.sock.debug:
			c.ClientMsg(dms, true)
		case ms := <-c.sock.infoLog:
			c.ClientMsg(ms, false)
		case msg := <-c.sock.messages:
			if msg != nil {
				msgHandler(msg)
			}
		}
	}
}
