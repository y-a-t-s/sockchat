package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"nhooyr.io/websocket"
)

type ChatSocket struct {
	Conn     *websocket.Conn
	Context  context.Context
	Received chan ChatMessage
	URL      *url.URL
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

func (sock *ChatSocket) Fetch() *ChatSocket {
	if sock.Conn == nil {
		return sock.Connect().Fetch()
	}

	for {
		_, msg, err := sock.Conn.Read(sock.Context)
		if err != nil {
			sock.ClientMsg("Failed to read from socket.")
			return sock.Connect().Fetch()
		}

		sockMsg := SocketMessage{}
		err = json.Unmarshal(msg, &sockMsg)
		if err != nil {
			log.Fatal("Failed to parse server response.")
		}

		for _, m := range sockMsg.Messages {
			sock.Received <- m
		}
	}
}

func NewSocket() *ChatSocket {
	ctx := context.Background()
	host, port := os.Getenv("SC_HOST"), os.Getenv("SC_PORT")

	sockUrl, err := url.Parse(fmt.Sprintf("wss://%s:%s/chat.ws", host, port))
	if err != nil {
		log.Fatal("Failed to parse socket URL.")
	}

	sock := &ChatSocket{
		Conn:     nil,
		Context:  ctx,
		Received: make(chan ChatMessage, 1024),
		URL:      sockUrl,
	}

	return sock
}

func (sock *ChatSocket) Connect() *ChatSocket {
	client := http.Client{
		Timeout: 0,
	}

	conn, _, err := websocket.Dial(sock.Context, sock.URL.String(), &websocket.DialOptions{
		HTTPClient: &client,
		HTTPHeader: getHeaders(),
	})
	if err != nil {
		sock.Conn = nil
		sock.ClientMsg("Failed to connect. Retrying...")
		return sock.Connect()
	}
	sock.ClientMsg("Connected.\n")

	// Disable read size limit.
	conn.SetReadLimit(-1)

	sock.Conn = conn
	// Let caller defer close.

	// Send join channel message. Raw bytes; not JSON encoded.
	err = sock.Conn.Write(sock.Context, websocket.MessageText, []byte("/join 1"))
	if err != nil {
		log.Panic("Failed to send join message.")
	}

	return sock
}
