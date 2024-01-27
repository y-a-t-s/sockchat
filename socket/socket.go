package socket

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cretz/bine/tor"
	"github.com/gorilla/websocket"
)

type ChatSocket struct {
	Channels MsgChannels
	conn     *websocket.Conn
	room     []byte
	tor      *tor.Tor
	url      url.URL
}

type MsgChannels struct {
	Messages       chan ChatMessage // Incoming msg feed
	Outgoing       chan []byte      // Outgoing queue
	serverResponse chan []byte      // Server response data
	Users          chan User        // Received user data
	UserQuery      chan UserQuery
}

func getHeaders() map[string][]string {
	headers := make(map[string][]string)
	headers["Cookie"] = []string{os.Args[1]}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"}

	return headers
}

func Init() *ChatSocket {
	sock := newSocket()
	sock.connect()

	go sock.fetch()
	go sock.write()

	go sock.responseHandler()
	go sock.UserHandler()

	return sock
}

func newSocket() *ChatSocket {
	su := func() *url.URL {
		host, port := strings.TrimRight(os.Getenv("SC_HOST"), "/"), os.Getenv("SC_PORT")
		// Assemble url to chat.ws with appropriate domain and port.
		// Note: os.Getenv returns the port as a string, so no %d in the format string.
		su, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
		if err != nil {
			log.Fatal("Failed to parse socket URL.\n", err)
		}

		return su
	}()

	room := os.Getenv("SC_DEF_ROOM")
	if room == "" {
		log.Panic("SC_DEF_ROOM not defined. Check .env")
	}

	sock := &ChatSocket{
		Channels: MsgChannels{
			Messages:       make(chan ChatMessage, 1024),
			Outgoing:       make(chan []byte, 32),
			serverResponse: make(chan []byte, 256),
			Users:          make(chan User, 1024),
			UserQuery:      make(chan UserQuery, 128),
		},
		conn: nil,
		room: []byte(room),
		tor:  nil,
		url:  *su,
	}

	return sock
}

func (sock *ChatSocket) TryClose() bool {
	if sock.conn == nil {
		return false
	}

	sock.conn.Close()
	sock.conn = nil

	return true
}

func (sock *ChatSocket) connect() *ChatSocket {
	// Close before re-opening if needed.
	sock.TryClose()

	getTorDialer := func() tor.Dialer {
		if sock.tor == nil {
			log.Print("Starting tor...")
			t, err := tor.Start(nil, nil)
			if err != nil {
				log.Fatal(err)
			}
			sock.tor = t
		}

		td, err := sock.tor.Dialer(context.Background(), nil)
		if err != nil {
			log.Fatal(err)
		}

		return *td
	}

	sock.ClientMsg("Opening socket...")
	dialer := func() websocket.Dialer {
		// Set handshake timeout to 15 mins.
		timeout, err := time.ParseDuration("15m")
		if err != nil {
			log.Panic("Failed to parse timeout duration string.\n", err)
		}

		wd := websocket.Dialer{
			EnableCompression: true,
			HandshakeTimeout:  timeout,
		}

		host := sock.url.Hostname()
		if os.Getenv("SC_USE_TOR") == "1" || strings.HasSuffix(host, ".onion") {
			td := getTorDialer()
			wd.NetDialContext = td.DialContext
		}

		return wd
	}()

	conn, _, err := dialer.Dial(sock.url.String(), getHeaders())
	if err != nil {
		sock.ClientMsg("Failed to connect. Retrying...")
		return sock.connect()
	}
	sock.ClientMsg("Connected.\n")

	sock.conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	sock.Channels.Outgoing <- []byte(fmt.Sprintf("/join %s", sock.room))

	return sock
}

func (sock *ChatSocket) fetch() *ChatSocket {
	for {
		if sock.conn == nil {
			sock.connect()
		}

		_, msg, err := sock.conn.ReadMessage()
		if err != nil {
			sock.ClientMsg("Failed to read from socket.\n")
			sock.connect()
		}

		sock.Channels.serverResponse <- msg
	}
}

func (sock *ChatSocket) write() {
	joinRE := regexp.MustCompile(`^/join \d+`)

	for {
		// Trim unnecessary whitespace.
		msg := bytes.TrimSpace(<-sock.Channels.Outgoing)
		// Ignore empty messages.
		if len(msg) == 0 {
			continue
		}

		if joinRE.Match(msg) {
			sock.room = bytes.Split(msg, []byte(" "))[1]
		}

		if err := sock.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			sock.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		}
	}
}
