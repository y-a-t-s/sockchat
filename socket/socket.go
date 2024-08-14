package socket

import (
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

// Abstraction layer for accessing socket data.
type Socket interface {
	ClientMsg(msg string)
	ClientName() (string, error)
	CloseAll()
	ReadMsg() ChatMessage
	QueryUser(id string) string
	Send(msg interface{}) error
	Start(ctx context.Context)
}

type channels struct {
	messages  chan ChatMessage // Incoming msg feed
	outgoing  chan []byte      // Outgoing msg feed
	userQuery chan UserQuery   // Used to query user data

	incoming chan []byte // Server response data
	users    chan User   // Received user data
}

type sock struct {
	*websocket.Conn
	channels

	client   string
	clientID int
	cookies  []string
	readOnly bool
	room     uint
	url      url.URL

	proxy socksProxy
}

func NewSocket(ctx context.Context, cfg config.Config) (Socket, error) {
	// Split the protocol part from addresses in the config, if present.
	splitProtocol := func(addr string) (string, string, error) {
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

	_, host, err := splitProtocol(cfg.Host)
	if err != nil {
		return nil, err
	}
	// Assemble url to chat.ws with appropriate domain and port.
	hostUrl, err := url.Parse(fmt.Sprintf("wss://%s:%d/chat.ws", host, cfg.Port))
	if err != nil {
		return nil, err
	}

	s := &sock{
		Conn: nil,
		channels: channels{
			messages:  make(chan ChatMessage, 1024),
			outgoing:  make(chan []byte, 16),
			userQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		clientID: cfg.UserID,
		client:   "",
		cookies:  cfg.Args,
		readOnly: cfg.ReadOnly,
		room:     cfg.Room,
		url:      *hostUrl,
		proxy:    socksProxy{},
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

	return s, nil
}

func (s *sock) ClientMsg(msg string) {
	s.messages <- ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageID:   0,
		MessageDate: time.Now().Unix(),
		MessageRaw:  msg,
	}
}

func (s *sock) ClientName() (string, error) {
	if s.client == "" {
		return "", errors.New("Client's ID does not have an entry in the user table yet.")
	}

	return s.client, nil
}

func (s *sock) CloseAll() {
	if s.Conn != nil {
		s.Close()
		s.Conn = nil
	}
	s.proxy.stopTor()
}

func (s *sock) ReadMsg() ChatMessage {
	msg := <-s.messages
	return msg
}

func (s *sock) Send(msg interface{}) error {
	switch msg.(type) {
	case string:
		s.outgoing <- []byte(msg.(string))
	case []byte:
		s.outgoing <- msg.([]byte)
	default:
		return errors.ErrUnsupported
	}

	return nil
}

func (s *sock) Start(ctx context.Context) {
	s.connect(ctx)
	s.startWorkers(ctx)
}

// Create WebSocket connection to the server.
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

	s.ClientMsg("Opening socket...")

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

	conn, _, err = wd.Dial(s.url.String(), map[string][]string{
		"Cookie":     s.cookies,
		"User-Agent": userAgent,
	})
	if err != nil {
		s.ClientMsg("Failed to connect.")
		return
	}
	s.ClientMsg("Connected.\n")

	// If in read-only mode, temporarily start the message writer.
	// Required to send /join message.
	if s.readOnly {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go s.msgWriter(ctx)
	}
	// Send /join message for desired room.
	s.Send(fmt.Sprintf("/join %d", s.room))

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

// WebSocket msg writing wrapper.
// Accepts []byte or string.
func (s *sock) write(msg interface{}) error {
	if s.Conn == nil {
		return errors.New("Connection is closed.")
	}

	var out []byte
	switch msg.(type) {
	case []byte:
		out = msg.([]byte)
	case string:
		out = []byte(msg.(string))
	}

	if err := s.WriteMessage(websocket.TextMessage, out); err != nil {
		s.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		return err
	}

	return nil
}
