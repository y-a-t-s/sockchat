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

	"golang.org/x/net/websocket"
)

type ChatSocket struct {
	Channels MsgChannels
	Conn     *websocket.Conn
	Room     []byte
	URL      url.URL
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

	go sock.fetch()
	go sock.write()

	go sock.responseHandler()
	go sock.UserHandler()

	return sock
}

func newSocket() *ChatSocket {
	host, port := strings.TrimRight(os.Getenv("SC_HOST"), "/"), os.Getenv("SC_PORT")

	sockUrl, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
	if err != nil {
		log.Fatal("Failed to parse socket URL.")
	}

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
		Conn: nil,
		Room: []byte(room),
		URL:  *sockUrl,
	}

	return sock
}

func (sock *ChatSocket) connect() *ChatSocket {
	if sock.Conn != nil {
		sock.Conn.Close()
		sock.Conn = nil
	}

	host, port := strings.TrimRight(os.Getenv("SC_HOST"), "/"), os.Getenv("SC_PORT")

	sockUrl, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
	if err != nil {
		log.Fatal("Failed to parse socket URL.")
	}
	originUrl, err := url.Parse(fmt.Sprintf("https://%s", host))
	if err != nil {
		log.Fatal("Failed to parse origin URL.")
	}

	sock.ClientMsg("Opening socket...")
	cfg, err := websocket.NewConfig(sockUrl.String(), originUrl.String())
	if err != nil {
		log.Fatal("Failed to create config for WebSocket connection.\n", err)
	}
	cfg.Header = getHeaders()

	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		sock.ClientMsg("Failed to connect. Retrying...")
		return sock.connect()
	}

	sock.ClientMsg("Connected.\n")

	sock.Conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	sock.Channels.Outgoing <- []byte(fmt.Sprintf("/join %s", sock.Room))

	return sock
}

func (sock *ChatSocket) fetch() *ChatSocket {
	if sock.Conn == nil {
		return sock.connect().fetch()
	}

	for {
		// Set timeout for read and write to 15 mins from now.
		t := time.Now()
		sock.Conn.SetDeadline(t.Add(time.Minute * 15))

		msg := (&bytes.Buffer{}).Bytes()
		err := websocket.Message.Receive(sock.Conn, &msg)
		if err != nil {
			sock.ClientMsg("Failed to read from socket.\n")
			if sock.Conn != nil {
				sock.Conn.Close()
				sock.Conn = nil
			}
			return sock.connect().fetch()
		}

		sock.Channels.serverResponse <- msg
	}
}

func (sock *ChatSocket) write() {
	for {
		msg := <-sock.Channels.Outgoing

		// Trim unnecessary whitespace.
		msg = bytes.TrimSpace(msg)
		// Ignore empty messages.
		if len(msg) == 0 {
			continue
		}

		if regexp.MustCompile(`^/join \d+$`).Match(msg) {
			// Update room ID with each join message.
			// Important for reconnecting to same channel if dropped.
			sock.Room = bytes.Split(msg, []byte(" "))[1]
		}

		if _, err := sock.Conn.Write(msg); err != nil {
			sock.ClientMsg(fmt.Sprintf("Failed to send: %s", msg))
		}
	}
}
