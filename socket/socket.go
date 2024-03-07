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

type Socket struct {
	*websocket.Conn
	torInst
	channels

	clientName string
	cookies    []string
	room       uint
	url        *url.URL
	readOnly   bool
}

type channels struct {
	Messages  chan ChatMessage // Incoming msg feed
	Outgoing  chan []byte      // Outgoing msg feed
	UserQuery chan UserQuery   // Used to query user data

	incoming chan []byte // Server response data
	users    chan User   // Received user data
}

type Chat interface {
	ClientMsg(msg string)
	GetClientName() string
	GetIncoming() ChatMessage
	QueryUser(id string) string
	Send(msg interface{})
	Start() error
}

func NewSocket(cfg config.Config) (*Socket, error) {
	parseHost := func() (*url.URL, error) {
		// SC_HOST might start with the protocol or have a trailing /, so isolate the domain.
		tmp := regexp.MustCompile(`(https?://)?([\w.]+)/?`).FindStringSubmatch(cfg.Host)
		if len(tmp) < 3 {
			return nil, errors.New("Failed to parse SC_HOST from env.")
		}
		host, port := tmp[2], cfg.Port

		// Assemble url to chat.ws with appropriate domain and port.
		su, err := url.Parse(fmt.Sprintf("wss://%s:%d/chat.ws", host, port))
		if err != nil {
			panic(err)
		}

		return su, nil
	}

	hostUrl, err := parseHost()
	if err != nil {
		return nil, err
	}
	sock := &Socket{
		nil,
		torInst{},
		channels{
			Messages:  make(chan ChatMessage, 1024),
			Outgoing:  make(chan []byte, 16),
			UserQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		"",
		cfg.Args,
		cfg.Room,
		hostUrl,
		cfg.ReadOnly,
	}

	if cfg.UseTor || strings.HasSuffix(sock.url.Hostname(), ".onion") {
		sock.tor = startTor()
		sock.getTorCtx()
	}

	return sock, nil
}

func (sock *Socket) ClientMsg(msg string) {
	cm := ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageID:   0,
		MessageDate: time.Now().Unix(),
		MessageRaw:  msg,
	}

	sock.Messages <- cm
}

func (sock *Socket) GetClientName() string {
	return sock.clientName
}

func (sock *Socket) GetIncoming() ChatMessage {
	msg := <-sock.Messages
	return msg
}

func (sock *Socket) Start() error {
	go func() {
		sock.connect()
		defer sock.CloseAll()
		defer sock.stopTor()

		sock.responseHandler()
	}()

	return nil
}

func (sock *Socket) Send(msg interface{}) {
	switch msg.(type) {
	case string:
		sock.Outgoing <- []byte(msg.(string))
	case []byte:
		sock.Outgoing <- msg.([]byte)
	}
}

func (sock *Socket) newDialer() (websocket.Dialer, error) {
	// Set handshake timeout to 15 mins.
	timeout, err := time.ParseDuration("15m")
	if err != nil {
		return websocket.Dialer{}, err
	}

	wd := websocket.Dialer{
		EnableCompression: true,
		HandshakeTimeout:  timeout,
	}
	if sock.tor != nil {
		// Dial socket through Tor proxy context.
		wd.NetDialContext = sock.torCtx
	}

	return wd, nil
}

func (sock *Socket) connect() error {
	sock.CloseAll()

	sock.ClientMsg("Opening socket...")

	dialer, err := sock.newDialer()
	if err != nil {
		return err
	}
	conn, _, err := dialer.Dial(sock.url.String(), map[string][]string{
		"Cookie":     sock.cookies,
		"User-Agent": []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"},
	})
	if err != nil {
		sock.ClientMsg("Failed to connect. Retrying...")
		return sock.connect()
	}
	sock.ClientMsg("Connected.\n")

	sock.Conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	// Writing directly to avoid requiring the msgWriter routine, which may not be running.
	err = sock.write([]byte(fmt.Sprintf("/join %d", sock.room)))
	if err != nil {
		sock.ClientMsg(fmt.Sprintf("Failed to send join message."))
	}

	return nil
}

func (sock *Socket) write(msg []byte) error {
	err := sock.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		sock.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		return err
	}

	return nil
}

func (sock *Socket) CloseAll() bool {
	if sock.Conn == nil {
		return false
	}

	sock.Close()
	sock.Conn = nil

	return true
}
