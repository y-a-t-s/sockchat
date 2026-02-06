package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"y-a-t-s/sockchat/config"

	"github.com/gen2brain/beeep"
)

type Chat struct {
	*sock

	Errs    chan error
	Feeder  feeder
	History chan chan Message
}

func NewChat(ctx context.Context, cfg config.Config) (*Chat, error) {
	s, err := newSocket(ctx, cfg)
	if err != nil {
		return nil, err
	}

	c := &Chat{
		sock:    s,
		Errs:    s.errLog,
		History: make(chan chan Message, 1),
		Feeder:  newFeeder(ctx),
	}

	return c, nil
}

func (c *Chat) Reconnect(ctx context.Context) error {
	return c.sock.connect(ctx)
}

func (c *Chat) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel()
		c.sock.start(ctx)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		c.router(ctx)
		c.stop()
	}()

	wg.Wait()
}

func (c *Chat) recordHistory(feed <-chan *Message) chan chan Message {
	out := make(chan chan Message, 1)

	var prevID uint32 // ID of previously processed msg.

	hist := make(chan *Message, HIST_LEN)
	editHist := func(msg *Message) {
		close(hist)

		hc, nc := make(chan Message, HIST_LEN), make(chan *Message, HIST_LEN)
		for hm := range hist {
			if msg.MessageID == hm.MessageID {
				hm.Release()
				hm = msg
			}

			hc <- *hm
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
			// Better safe than sorry.
			case msg == nil:
				continue
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

func NewErrLog() (*os.File, error) {
	cfgDir, err := config.ConfigDir()
	if err != nil {
		return nil, err
	}
	errLogDir := filepath.Join(cfgDir, "error_logs")
	if err = os.Mkdir(errLogDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	logPath := filepath.Join(errLogDir, fmt.Sprintf("%s_err.log", time.Now().Format(_DATE_FMT)))
	return os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func (c *Chat) router(ctx context.Context) {
	histFeed := make(chan *Message, HIST_LEN)
	defer close(histFeed)
	hist := c.recordHistory(histFeed)

	if c.Cfg.Logger {
		logFeed := c.Feeder.Feed()
		err := startLogger(logFeed.Feed)
		if err != nil {
			panic(err)
		}
	}

	var reply struct {
		ID   uint32
		Date int64
	}

	msgHandler := func(msg *Message) {
		if msg == nil {
			return
		}

		if c.Users.ClientName() != "" && strings.Contains(msg.MessageRaw, fmt.Sprintf("@%s", c.Users.ClientName())) {
			msg.IsMention = true

			date := msg.MessageDate
			if msg.MessageEditDate > 0 {
				date = msg.MessageEditDate
			}

			if date >= reply.Date && msg.MessageID != reply.ID {
				reply.ID = msg.MessageID
				reply.Date = date

				beeep.Notify(fmt.Sprintf("Reply from @%s", msg.Author.Username), msg.MessageRaw, "")
			}
		}

		c.Feeder.Send(msg)
		histFeed <- msg
	}

	errFile, err := NewErrLog()
	if err != nil {
		panic(err)
	}
	defer errFile.Close()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case dms := <-c.sock.debug:
				c.ClientMsg(dms, true)
			case ms := <-c.sock.infoLog:
				c.ClientMsg(ms, false)
			case err := <-c.sock.errLog:
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(errFile, err)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		// Not directly assigned to help sync with new msgs.
		case hc := <-hist:
			c.History <- hc
		case msg := <-c.sock.messages:
			if msg != nil {
				msgHandler(msg)
			}
		}
	}
}
