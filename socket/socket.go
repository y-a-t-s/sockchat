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
	Received chan *ChatMessage
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

func (sock *ChatSocket) Fetch() {
	for {
		_, msg, err := sock.Conn.Read(sock.Context)
		if err != nil {
			log.Fatal("Failed to read from socket.")
		}

		sockMsg := &SocketMessage{}
		err = json.Unmarshal(msg, sockMsg)
		if err != nil {
			log.Fatal("Failed to parse server response.")
		}

		for _, m := range sockMsg.Messages {
			sock.Received <- &m
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
	client := &http.Client{
		Timeout: 0,
	}

	conn, _, err := websocket.Dial(ctx, sockUrl.String(), &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: getHeaders(),
	})
	if err != nil {
		log.Fatal("Failed to connect.", err)
	}

	// Disable read size limit.
	conn.SetReadLimit(-1)

	sock := &ChatSocket{
		Conn:     conn,
		Context:  ctx,
		Received: make(chan *ChatMessage, 1024),
	}
	sock.ClientMsg("Connected.")

	return sock
}

func (sock *ChatSocket) Connect() *ChatSocket {

	// Send join channel message. Raw bytes; not JSON encoded.
	err := sock.Conn.Write(sock.Context, websocket.MessageText, []byte("/join 1"))
	if err != nil {
		log.Fatal("Failed to send join message.")
		// return err
	}

	return sock
}
