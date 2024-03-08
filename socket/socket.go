package socket

import (
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
	GetClientName() string
	GetIncoming() ChatMessage
	QueryUser(id string) string
	Send(msg interface{}) error
	Start()
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

	clientName string
	cookies    []string
	readOnly   bool
	room       uint
	url        url.URL
}

func NewSocket(cfg config.Config) (*sock, error) {
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
	sock := &sock{
		Conn:    nil,
		torInst: torInst{},
		channels: channels{
			messages:  make(chan ChatMessage, 1024),
			outgoing:  make(chan []byte, 16),
			userQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		clientName: "",
		cookies:    cfg.Args,
		readOnly:   cfg.ReadOnly,
		room:       cfg.Room,
		url:        *hostUrl,
	}

	if cfg.UseTor || strings.HasSuffix(sock.url.Hostname(), ".onion") {
		sock.tor = startTor()
		sock.getTorCtx()
	}

	return sock, nil
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

func (s *sock) Start() {
	go func() {
		_, err := s.connect()
		if err != nil {
			panic(err)
		}
		defer s.CloseAll()
		defer s.stopTor()

		s.responseHandler()
	}()
}

func (s *sock) newDialer() (websocket.Dialer, error) {
	// Set handshake timeout to 15 mins.
	timeout, err := time.ParseDuration("15m")
	if err != nil {
		return websocket.Dialer{}, err
	}

	wd := websocket.Dialer{
		EnableCompression: true,
		HandshakeTimeout:  timeout,
	}
	if s.tor != nil {
		// Dial socket through Tor proxy context.
		wd.NetDialContext = s.torCtx
	}

	return wd, nil
}

func (s *sock) connect() (*websocket.Conn, error) {
	s.CloseAll()

	s.ClientMsg("Opening socket...")

	dialer, err := s.newDialer()
	if err != nil {
		return nil, err
	}
	conn, _, err := dialer.Dial(s.url.String(), map[string][]string{
		"Cookie":     s.cookies,
		"User-Agent": []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"},
	})
	if err != nil {
		s.ClientMsg("Failed to connect. Retrying...")
		return s.connect()
	}
	s.ClientMsg("Connected.\n")

	s.Conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	// Writing directly to avoid requiring the msgWriter routine, which may not be running.
	err = s.write([]byte(fmt.Sprintf("/join %d", s.room)))
	if err != nil {
		s.ClientMsg(fmt.Sprintf("Failed to send join message."))
	}

	return conn, nil
}

func (s *sock) write(msg []byte) error {
	err := s.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		s.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		return err
	}

	return nil
}

func (s *sock) CloseAll() bool {
	if s.Conn == nil {
		return false
	}

	s.Close()
	s.Conn = nil

	return true
}
