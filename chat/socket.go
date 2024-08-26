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

// Max chat history length.
const HIST_LEN = 512

// User-Agent string for headers.
const USER_AGENT = "Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"

type sock struct {
	*websocket.Conn

	messages chan *ChatMessage

	Out chan string

	cookies  []string
	readOnly bool
	room     uint
	url      *url.URL
	proxy    *socksProxy
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

func newSocket(ctx context.Context, cfg config.Config) (s *sock, err error) {
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

	cookies := cfg.Args
	if !strings.Contains(cookies[0], "xf_session=") {
		cookies[0] = "xf_session=; " + cookies[0]
	}

	s = &sock{
		Conn:     nil,
		messages: make(chan *ChatMessage, 1024),
		Out:      make(chan string, 16),
		cookies:  cookies,
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
		p, err := newSocksDialer(addr)
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
	if s.proxy != nil {
		s.proxy.stopTor()
	}
	close(s.Out)
}

func (s *sock) connect(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Close if necessary before connecting.
	s.disconnect()

	// Create new WebSocket dialer, routing through any applicable proxies.
	wd := websocket.Dialer{
		EnableCompression: true,
		// Set handshake timeout to 5 mins.
		HandshakeTimeout: time.Minute * 5,
	}
	if s.proxy != nil {
		// Dial socket through proxy context.
		wd.NetDialContext = s.proxy.DialContext
	}

	// UA defined up here to make the redundant slice warning fuck off.
	ua := []string{USER_AGENT}
	conn, _, err := wd.DialContext(ctx, s.url.String(), map[string][]string{
		"Cookie":     s.cookies,
		"User-Agent": ua,
	})
	if err != nil {
		return err
	}

	conn.EnableWriteCompression(true)
	// Send /join message for desired room.
	s.Out <- fmt.Sprintf("/join %d", s.room)

	// Set s.conn at the end to avoid early access.
	s.Conn = conn
	return nil
}

func (s *sock) disconnect() {
	if s.Conn != nil {
		s.Conn.Close()
		s.Conn = nil
	}
}

// Tries reconnecting 8 times.
func (s *sock) reconnect(ctx context.Context) error {
	for i := 0; i < 8; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := s.connect(ctx)
		if err == nil {
			return nil
		}
	}

	return errors.New("Reconnect failed.")
}

// WebSocket msg writing wrapper. Not thread safe by itself.
// Accepts []byte or string.
func (s *sock) write(msg string) error {
	if s.Conn == nil {
		return errors.New("Socket is closed.")
	}

	out := bytes.TrimSpace([]byte(msg))
	if err := s.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	return nil
}

func (c *Chat) msgReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if c.sock.Conn == nil {
			c.ClientMsg("Opening socket...")
			err := c.sock.reconnect(ctx)
			if err != nil {
				c.ClientMsg("Failed to connect 8 times. Waiting 15 seconds.")
				time.Sleep(time.Second * 15)
				continue
			}
			c.ClientMsg("Connected.\n")
			continue
		}

		_, msg, err := c.sock.ReadMessage()
		if err != nil {
			c.ClientMsg("Failed to read from socket.\n")
			c.sock.Conn = nil
			continue
		}

		err = c.parseResponse(msg)
		if err != nil {
			errMsg := err.Error()
			c.ClientMsg(errMsg)

			if strings.Contains(errMsg, "cannot join") {
				tk, err := c.kf.RefreshSession()
				if err != nil {
					continue
				}
				c.sock.cookies[0] = regexp.MustCompile(`xf_session=.*`).
					ReplaceAllString(c.sock.cookies[0], tk)
				c.sock.disconnect()
			}
		}
	}
}
