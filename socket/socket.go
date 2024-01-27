package socket

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type socket struct {
	conn *websocket.Conn
	room []byte
	torInst
	url url.URL
}

type Session struct {
	*channels
	*socket
}

type channels struct {
	Messages  chan ChatMessage // Incoming msg feed
	Outgoing  chan []byte      // Outgoing msg feed
	UserQuery chan UserQuery   // Used to query user data

	incoming chan []byte // Server response data
	users    chan User   // Received user data
}

func NewSession() *Session {
	ssn := &Session{
		channels: &channels{
			Messages:  make(chan ChatMessage, 1024),
			Outgoing:  make(chan []byte, 16),
			UserQuery: make(chan UserQuery, 16),
			incoming:  make(chan []byte, 1024),
			users:     make(chan User, 1024),
		},
		socket: newSocket(),
	}

	go func() {
		ssn.connect()
		defer ssn.TryClose()
		defer ssn.stopTor()

		ssn.responseHandler()
	}()

	return ssn
}

func newSocket() *socket {
	su := func() url.URL {
		host, port := strings.TrimRight(os.Getenv("SC_HOST"), "/"), os.Getenv("SC_PORT")
		// Assemble url to chat.ws with appropriate domain and port.
		// Note: os.Getenv returns the port as a string, so no %d in the format string.
		su, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
		if err != nil {
			log.Fatal("Failed to parse socket URL.\n", err)
		}

		return *su
	}()

	room := os.Getenv("SC_DEF_ROOM")
	if room == "" {
		log.Panic("SC_DEF_ROOM not defined. Check .env")
	}

	return &socket{
		conn: nil,
		room: []byte(room),
		url:  su,
	}
}

func (ssn *Session) connect() *Session {
	// Close before re-opening if needed.
	ssn.TryClose()

	ssn.ClientMsg("Opening socket...")
	wd := func() websocket.Dialer {
		// Set handshake timeout to 15 mins.
		timeout, err := time.ParseDuration("15m")
		if err != nil {
			log.Panic("Failed to parse timeout duration string.\n", err)
		}

		wd := websocket.Dialer{
			EnableCompression: true,
			HandshakeTimeout:  timeout,
		}

		// If SC_HOST is the .onion domain, the value of SC_USE_TOR is irrelevant.
		if os.Getenv("SC_USE_TOR") == "1" || strings.HasSuffix(ssn.url.Hostname(), ".onion") {
			if ssn.tor == nil {
				ssn.tor = startTor()
			}
			ssn.getTorCtx()
			// Dial socket through Tor proxy context.
			wd.NetDialContext = ssn.torCtx
		}

		return wd
	}()

	conn, _, err := wd.Dial(ssn.url.String(), map[string][]string{
		"Cookie":     []string{os.Args[1]},
		"User-Agent": []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"},
	})
	if err != nil {
		ssn.ClientMsg("Failed to connect. Retrying...")
		return ssn.connect()
	}
	ssn.ClientMsg("Connected.\n")

	ssn.conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	ssn.Outgoing <- []byte(fmt.Sprintf("/join %s", ssn.room))

	return ssn
}

func (ssn *Session) fetch() {
	for {
		if ssn.conn == nil {
			ssn.connect()
		}

		_, msg, err := ssn.conn.ReadMessage()
		if err != nil {
			ssn.ClientMsg("Failed to read from socket.\n")
			ssn.connect()
		}

		ssn.incoming <- msg
	}
}

func (ssn *Session) write() {
	joinRE := regexp.MustCompile(`^/join \d+$`)
	for {
		// Trim unnecessary whitespace.
		msg := bytes.TrimSpace(<-ssn.Outgoing)
		// Ignore empty messages.
		if len(msg) == 0 {
			continue
		}

		// Update selected room if /join message was sent.
		if joinRE.Match(msg) {
			ssn.room = bytes.Split(msg, []byte(" "))[1]
		}

		if err := ssn.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			ssn.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		}
	}
}

func (sock *socket) TryClose() bool {
	if sock.conn == nil {
		return false
	}

	sock.conn.Close()
	sock.conn = nil

	return true
}
