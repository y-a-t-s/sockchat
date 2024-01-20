package socket

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type ChatSocket struct {
	Channels MsgChannels
	Conn     *websocket.Conn
	Room     string
	URL      url.URL
	Users    UserTable
}

type MsgChannels struct {
	Messages       chan ChatMessage // Incoming msg feed
	Outgoing       chan string      // Outgoing queue
	serverResponse chan []byte      // Server response data
	Users          chan User        // Received user data
}

type UserTable struct {
	UserMap map[string]string
	Mutex   sync.Mutex
}

func getHeaders() map[string][]string {
	headers := make(map[string][]string)
	headers["Cookie"] = []string{os.Args[1]}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"}

	return headers
}

func NewSocket() *ChatSocket {
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
			Outgoing:       make(chan string, 32),
			serverResponse: make(chan []byte, 256),
			Users:          make(chan User, 1024),
		},
		Conn: nil,
		Room: room,
		URL:  *sockUrl,
		Users: UserTable{
			UserMap: make(map[string]string),
		},
	}

	// Start up response handler.
	go sock.responseHandler()

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

	// Set timeout for read and write to 15 mins
	t := time.Now()
	conn.SetDeadline(t.Add(time.Minute * 15))

	sock.Conn = conn
	// Let caller defer close.

	// Send /join message for desired room.
	sock.Channels.Outgoing <- fmt.Sprintf("/join %s", sock.Room)
	sock.Write()

	return sock
}

func (sock *ChatSocket) Fetch() *ChatSocket {
	if sock.Conn == nil {
		return sock.connect().Fetch()
	}

	for {
		msg := (&bytes.Buffer{}).Bytes()
		err := websocket.Message.Receive(sock.Conn, &msg)
		if err != nil {
			log.Fatal(err)
			sock.ClientMsg("Failed to read from socket.\n")
			if sock.Conn != nil {
				sock.Conn.Close()
			}
			sock.Conn = nil
			return sock.connect().Fetch()
		}

		sock.Channels.serverResponse <- msg
	}
}

func (sock *ChatSocket) Write() *ChatSocket {
	// Wait until socket has reconnected when needed.
	for sock.Conn == nil {
	}

	msg := <-sock.Channels.Outgoing

	// Trim unnecessary whitespace.
	strings.TrimSpace(msg)
	// Ignore empty messages.
	if len(msg) == 0 {
		return sock
	}

	if regexp.MustCompile(`^/join \d+$`).MatchString(msg) {
		// Update room ID with each join message.
		// Important for reconnecting to same channel if dropped.
		sock.Room = strings.Split(msg, " ")[1]
	}

	if _, err := sock.Conn.Write([]byte(msg)); err != nil {
		sock.ClientMsg("Failed to send: " + msg)
	}

	return sock
}
