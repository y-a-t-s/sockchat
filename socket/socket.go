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

type Socket interface {
	ClientMsg(msg string)
	CloseAll()
	GetClientName() string
	GetIncoming() ChatMessage
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
	torInst
	channels

	clientID   int
	clientName string
	cookies    []string
	readOnly   bool
	room       uint
	url        url.URL
}

func NewSocket(ctx context.Context, cfg config.Config) (Socket, error) {
	parseHost := func() (*url.URL, error) {
		// Provided host might start with the protocol or have a trailing /, so isolate the domain.
		tmp := regexp.MustCompile(`(https?://)?([\w.]+)/?`).FindStringSubmatch(cfg.Host)
		if len(tmp) < 3 {
			return nil, errors.New("Failed to parse SC_HOST from env.")
		}
		host, port := tmp[2], cfg.Port

		// Assemble url to chat.ws with appropriate domain and port.
		su, err := url.Parse(fmt.Sprintf("wss://%s:%d/chat.ws", host, port))
		if err != nil {
			return nil, err
		}

		return su, nil
	}

	hostUrl, err := parseHost()
	if err != nil {
		return nil, err
	}
	s := &sock{
		Conn:    nil,
		torInst: torInst{},
		channels: channels{
			messages:  make(chan ChatMessage, 1024),
			outgoing:  make(chan []byte, 16),
			userQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		clientID:   cfg.UserID,
		clientName: "",
		cookies:    cfg.Args,
		readOnly:   cfg.ReadOnly,
		room:       cfg.Room,
		url:        *hostUrl,
	}

	if cfg.Tor || strings.HasSuffix(s.url.Hostname(), ".onion") {
		if err := s.startTor(ctx); err != nil {
			return nil, err
		}
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

func (s *sock) GetClientName() string {
	return s.clientName
}

func (s *sock) GetIncoming() ChatMessage {
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
	_, err := s.connect(ctx)
	if err != nil {
		panic(err)
	}

	go s.fetch(ctx)
	go s.userHandler(ctx)
	if !s.readOnly {
		go s.msgWriter(ctx)
	}
	s.responseHandler(ctx)
}

func (s *sock) CloseAll() {
	if s.Conn != nil {
		s.Close()
		s.Conn = nil
	}
	s.stopTor()
}

func (s *sock) connect(ctx context.Context) (*websocket.Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Close if necessary before reconnecting.
	if s.Conn != nil {
		s.Close()
		s.Conn = nil
	}

	s.ClientMsg("Opening socket...")

	dialer, err := s.newDialer(ctx)
	if err != nil {
		return nil, err
	}
	conn, _, err := dialer.Dial(s.url.String(), map[string][]string{
		"Cookie":     s.cookies,
		"User-Agent": []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"},
	})
	if err != nil {
		s.ClientMsg("Failed to connect. Retrying...")
		return s.connect(ctx)
	}
	s.ClientMsg("Connected.\n")

	s.Conn = conn

	// Send /join message for desired room.
	// Writing directly to avoid requiring the msgWriter routine, which may not be running.
	err = s.write([]byte(fmt.Sprintf("/join %d", s.room)))
	if err != nil {
		s.ClientMsg(fmt.Sprintf("Failed to send join message."))
	}

	return conn, nil
}

func (s *sock) newDialer(ctx context.Context) (websocket.Dialer, error) {
	// Set handshake timeout to 15 mins.
	timeout, err := time.ParseDuration("15m")
	if err != nil {
		return websocket.Dialer{}, err
	}

	wd := websocket.Dialer{
		EnableCompression: true,
		HandshakeTimeout:  timeout,
	}
	// s.Tor should only be non-nil when Tor is running.
	if s.Tor != nil {
		if s.proxy == nil {
			s.getTorProxy(ctx)
		}
		// Dial socket through Tor proxy context.
		wd.NetDialContext = s.proxy
	}

	return wd, nil
}

func (s *sock) write(msg []byte) error {
	if s.Conn == nil {
		return errors.New("Socket connection is nil.")
	}

	err := s.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		s.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		return err
	}

	return nil
}
