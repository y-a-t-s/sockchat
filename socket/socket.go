package socket

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"

	"nhooyr.io/websocket"
)

type ChatSocket struct {
	Channels MsgChannels
	Conn     *websocket.Conn
	Context  context.Context
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
	host, port := os.Getenv("SC_HOST"), os.Getenv("SC_PORT")

	headers := make(map[string][]string)
	headers["Cookie"] = []string{os.Args[1]}
	headers["Host"] = []string{fmt.Sprintf("%s:%s", host, port)}
	headers["Origin"] = []string{fmt.Sprintf("https://%s", host)}
	headers["User-Agent"] = []string{"Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"}

	return headers
}

func NewSocket() *ChatSocket {
	ctx := context.Background()
	host, port := os.Getenv("SC_HOST"), os.Getenv("SC_PORT")

	sockUrl, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
	if err != nil {
		log.Fatal("Failed to parse socket URL.")
	}

	sock := &ChatSocket{
		Channels: MsgChannels{
			Messages:       make(chan ChatMessage, 1024),
			Outgoing:       make(chan string, 32),
			serverResponse: make(chan []byte, 256),
			Users:          make(chan User, 1024),
		},
		Conn:    nil,
		Context: ctx,
		URL:     *sockUrl,
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
		sock.Conn.CloseNow()
		sock.Conn = nil
	}

	sock.ClientMsg("Opening socket...")
	conn, _, err := websocket.Dial(sock.Context, sock.URL.String(), &websocket.DialOptions{
		CompressionMode: websocket.CompressionContextTakeover,
		HTTPClient: &http.Client{
			Timeout: 0,
		},
		HTTPHeader: getHeaders(),
	})
	if err != nil {
		sock.ClientMsg("Failed to connect. Retrying...")
		return sock.connect()
	}
	sock.ClientMsg("Connected.\n")

	// Disable read size limit.
	conn.SetReadLimit(-1)

	sock.Conn = conn
	// Let caller defer close.

	// Send join channel message. Raw bytes; not JSON encoded. Joins default room set in .env
	room := os.Getenv("SC_DEF_ROOM")
	// Missing env var returns empty string
	if room == "" {
		log.Panic("SC_DEF_ROOM not defined. Check .env")
	}
	sock.Channels.Outgoing <- fmt.Sprintf("/join %s", room)
	sock.Write()

	return sock
}

func (sock *ChatSocket) Fetch() *ChatSocket {
	if sock.Conn == nil {
		return sock.connect().Fetch()
	}

	for {
		_, msg, err := sock.Conn.Read(sock.Context)
		if err != nil {
			sock.ClientMsg("Failed to read from socket.\n")
			sock.Conn.CloseNow()
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
	err := sock.Conn.Write(sock.Context, websocket.MessageText, []byte(msg))
	if err != nil {
		sock.ClientMsg("Failed to send: " + msg)
	}

	return sock
}
