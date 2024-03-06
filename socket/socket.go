package socket

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Socket struct {
	*websocket.Conn
	*torInst
	channels
	room []byte
	url  *url.URL
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
	GetIncoming() ChatMessage
	QueryUser(id string) string
	Send(msg []byte)
	Start() error
}

func NewSocket() (Socket, error) {
	parseHost := func() *url.URL {
		// SC_HOST might start with the protocol or have a trailing /, so isolate the domain.
		tmp := regexp.MustCompile(`(https?://)?([\w.]+)/?`).FindStringSubmatch(os.Getenv("SC_HOST"))
		if len(tmp) < 3 {
			panic("Failed to parse SC_HOST from env.")
		}
		host, port := tmp[2], os.Getenv("SC_PORT")

		// Assemble url to chat.ws with appropriate domain and port.
		// Note: os.Getenv returns the port as a string, so no %d in the format string.
		su, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
		if err != nil {
			panic(err)
		}

		return su
	}

	room := os.Getenv("SC_DEF_ROOM")
	if room == "" {
		panic("SC_DEF_ROOM not defined. Check .env")
	}

	sock := Socket{
		nil,
		nil,
		channels{
			Messages:  make(chan ChatMessage, 1024),
			Outgoing:  make(chan []byte, 16),
			UserQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		[]byte(room),
		parseHost(),
	}

	return sock, nil
}

func (sock Socket) ClientMsg(msg string) {
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

func (sock Socket) GetIncoming() ChatMessage {
	msg := <-sock.Messages
	return msg
}

func (sock Socket) Start() error {
	go func() {
		sock.connect()
		defer sock.CloseAll()
		defer sock.stopTor()

		sock.responseHandler()
	}()

	return nil
}

func (sock Socket) Send(msg []byte) {
	sock.Outgoing <- msg
}

func (sock *Socket) newDialer() (websocket.Dialer, error) {
	// Set handshake timeout to 15 mins.
	timeout, err := time.ParseDuration("15m")
	if err != nil {
		panic(err)
	}

	wd := websocket.Dialer{
		EnableCompression: true,
		HandshakeTimeout:  timeout,
	}

	// If SC_HOST is the .onion domain, the value of SC_USE_TOR is irrelevant.
	if os.Getenv("SC_USE_TOR") == "1" || strings.HasSuffix(sock.url.Hostname(), ".onion") {
		if sock.tor == nil {
			sock.tor = startTor()
		}
		sock.getTorCtx()
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
		"Cookie":     []string{os.Args[1]},
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
	err = sock.write([]byte(fmt.Sprintf("/join %s", sock.room)))
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
