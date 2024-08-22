package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"y-a-t-s/sockchat/config"

	"github.com/gorilla/websocket"
)

const HIST_LEN = 512

type sock struct {
	*websocket.Conn
	pool msgPool

	Messages chan *ChatMessage
	userData chan *User

	out chan string

	cookies  []string
	readOnly bool
	room     uint
	url      *url.URL

	proxy socksProxy
}

// Split the protocol part from addresses in the config, if present.
func splitProtocol(addr string) (string, string, error) {
	// FindStringSubmatch is used to capture the groups.
	// Index 0 is the full matching string with all groups.
	// The rest are numbered by the order of the opening parens.
	// Here, we want the last 2 groups (indexes 1 and 2, requiring length 3).
	tmp := regexp.MustCompile(`([\w-]+://)?([^/]+)`).FindStringSubmatch(addr)
	// At the very least, we need the hostname part (index 2).
	if len(tmp) < 3 || tmp[2] == "" {
		return "", "", errors.New(fmt.Sprintf("Failed to parse address: %s", addr))
	}

	return tmp[1], tmp[2], nil
}

func NewSocket(ctx context.Context, cfg config.Config) (s *sock, err error) {
	parseProxyAddr := func() (u *url.URL, err error) {
		proto, addr, err := splitProtocol(cfg.Proxy.Addr)
		if err != nil {
			return
		}
		// Fallback to socks5 if no protocol is given.
		if proto == "" {
			proto = "socks5"
		}
		u, err = url.Parse(fmt.Sprintf("%s://%s", proto, addr))
		if err != nil {
			return
		}

		// url.Parse collects any credentials in the URL to a *url.Userinfo.
		// If none are found, the pointer is nil.
		// Credentials in the URL take precedence over explicit ones in the config.
		if u.User == nil && cfg.Proxy.User != "" {
			// Create new &url.Userinfo with the explicit credentials.
			u.User = url.UserPassword(cfg.Proxy.User, cfg.Proxy.Pass)
		}

		return
	}

	s = &sock{
		Conn:     nil,
		pool:     newMsgPool(),
		Messages: make(chan *ChatMessage, 1024),
		userData: make(chan *User, 512),
		out:      make(chan string, 16),
		cookies:  cfg.Args,
		readOnly: cfg.ReadOnly,
		room:     cfg.Room,
	}

	err = s.setUrl(cfg.Host, uint16(cfg.Port))
	if err != nil {
		return
	}

	switch {
	case cfg.Tor, strings.HasSuffix(s.url.Hostname(), ".onion"):
		p, err := startTor(ctx)
		if err != nil {
			return nil, err
		}
		s.proxy = p
	case cfg.Proxy.Enabled:
		addr, err := parseProxyAddr()
		if err != nil {
			return nil, err
		}
		p, err := newSocksDialer(*addr)
		if err != nil {
			return nil, err
		}
		s.proxy = p
	}

	return
}

func (s *sock) setUrl(addr string, port uint16) error {
	_, host, err := splitProtocol(addr)
	if err != nil {
		return err
	}

	u, err := url.Parse(fmt.Sprintf("wss://%s:%d/chat.ws", host, port))
	if err != nil {
		return err
	}
	s.url = u

	return nil
}

func (s *sock) Stop() {
	if s.Conn != nil {
		s.Close()
		s.Conn = nil
	}
	s.proxy.stopTor()
}

func (s *sock) connect(ctx context.Context) (conn *websocket.Conn, err error) {
	select {
	case <-ctx.Done():
		err = ctx.Err()
		return
	default:
	}

	// User-Agent string for headers. Can't define a slice as a const.
	userAgent := []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"}

	// Close if necessary before reconnecting.
	if s.Conn != nil {
		s.Close()
		s.Conn = nil
	}

	s.Messages <- ClientMsg("Opening socket...")

	// Create new WebSocket dialer, routing through any applicable proxies.
	wd := websocket.Dialer{
		EnableCompression: true,
		// Set handshake timeout to 5 mins.
		HandshakeTimeout: time.Minute * 5,
	}
	if s.proxy.dialCtx != nil {
		// Dial socket through proxy context.
		wd.NetDialContext = s.proxy.dialCtx
	}

	conn, _, err = wd.DialContext(ctx, s.url.String(), map[string][]string{
		"Cookie":     s.cookies,
		"User-Agent": userAgent,
	})
	if err != nil {
		s.Messages <- ClientMsg("Failed to connect.")
		return
	}
	s.Messages <- ClientMsg("Connected.\n")

	conn.EnableWriteCompression(true)
	// Send /join message for desired room.
	s.out <- fmt.Sprintf("/join %d", s.room)

	// Set s.conn at the end to avoid early access.
	s.Conn = conn
	return
}

// Tries reconnecting 8 times.
func (s *sock) reconnect(ctx context.Context) (conn *websocket.Conn, err error) {
	for i := 0; i < 8; i++ {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
		}

		conn, err = s.connect(ctx)
		if err == nil {
			return
		}
	}

	err = errors.New("Reconnect failed.")
	return
}

// WebSocket msg writing wrapper. Not thread safe by itself.
// Accepts []byte or string.
func (s *sock) write(msg string) error {
	if s.Conn == nil {
		return errors.New("Connection is closed.")
	}

	out := bytes.TrimSpace([]byte(msg))
	if err := s.WriteMessage(websocket.TextMessage, out); err != nil {
		s.Messages <- ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		return err
	}

	return nil
}

func (s *sock) msgReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if s.Conn == nil {
			s.connect(ctx)
			continue
		}

		_, msg, err := s.ReadMessage()
		if err != nil {
			s.Messages <- ClientMsg("Failed to read from socket.\n")
			if _, err := s.reconnect(ctx); err != nil {
				s.Messages <- ClientMsg("Max retries reached. Waiting 15 seconds.")
				time.Sleep(time.Second * 15)
			}
			continue
		}

		s.parseResponse(msg)
	}
}

func (s *sock) router(ctx context.Context) {
	joinRE := regexp.MustCompile(`^/join \d+`)
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-s.out:
			if m == "" || !ok {
				continue
			}

			if !s.readOnly || joinRE.MatchString(m) {
				s.write(m)
			}
		}
	}
}

func (s *sock) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stopf := context.AfterFunc(ctx, func() {
		close(s.out)
		s.Stop()
	})
	defer stopf()

	go s.msgReader(ctx)
	s.router(ctx)
}
