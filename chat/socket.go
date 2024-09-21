package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"y-a-t-s/sockchat/config"

	"github.com/gorilla/websocket"
	"github.com/y-a-t-s/libkiwi"
)

const (
	// Max chat history length.
	HIST_LEN = 512
	// User-Agent string for headers.
	USER_AGENT = "Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"
)

type sockIO struct {
	debug   chan string
	errLog  chan error
	infoLog chan string

	chatJson chan []byte
	messages chan *Message
	users    chan *User
	Out      chan string

	once sync.Once
}

func newSockIO() *sockIO {
	return &sockIO{
		debug:    make(chan string, 16),
		errLog:   make(chan error, 8),
		infoLog:  make(chan string, 64),
		chatJson: make(chan []byte, 64),
		messages: make(chan *Message, HIST_LEN),
		users:    make(chan *User, 256),
		Out:      make(chan string, 16),
	}
}

func (sio *sockIO) CloseAll() {
	sio.once.Do(func() {
		// close(sio.errLog)
		// close(sio.infoLog)
		close(sio.debug)
		close(sio.chatJson)
		close(sio.messages)
		close(sio.users)
		close(sio.Out)
	})
}

type sock struct {
	*websocket.Conn
	*sockIO

	proxy *socksProxy
	url   *url.URL

	cfg config.Config
	kf  *libkiwi.KF
}

// Split the protocol part from addresses in the config, if present.
func parseHost(addr string) (*url.URL, error) {
	if !strings.Contains(addr, "://") {
		addr = "https://" + addr
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	return &url.URL{
		Scheme: u.Scheme,
		Host:   u.Hostname(),
	}, nil
}

func newSocket(ctx context.Context, cfg config.Config) (*sock, error) {
	s := &sock{
		sockIO: newSockIO(),
		cfg:    cfg,
	}

	err := s.setUrl(cfg.Host, uint16(cfg.Port))
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(s.url.Hostname(), ".onion") {
		cfg.Tor.Enabled = true
		cfg.Tor.Clearnet = false
	}

	switch {
	case cfg.Tor.Enabled:
		// Set socket URL to onion domain if desired.
		if !cfg.Tor.Clearnet {
			err = s.setUrl(cfg.Tor.Onion, uint16(cfg.Port))
			if err != nil {
				return nil, err
			}

			log.Printf("Connecting to onion service: %s\nMake sure this domain is correct.\n", s.url.Hostname())
			time.Sleep(3 * time.Second)
		}

		s.proxy, err = startTor(ctx)
		if err != nil {
			return nil, err
		}
	case cfg.Proxy.Enabled:
		s.proxy, err = newSocksDialer(cfg)
		if err != nil {
			return nil, err
		}
	}

	hc := http.Client{}

	// Will be default transport if s.proxy is nil.
	if s.proxy != nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DialContext = s.proxy.DialContext
		hc.Transport = tr
	}

	kf, err := libkiwi.NewKF(hc, s.url.Hostname(), cfg.Cookies)
	if err != nil {
		return nil, err
	}
	s.kf = kf

	return s, nil
}

func (s *sock) setUrl(addr string, port uint16) error {
	host, err := parseHost(addr)
	if err != nil {
		return err
	}

	u, err := url.Parse(fmt.Sprintf("wss://%s:%d/chat.ws", host.Hostname(), port))
	if err != nil {
		return err
	}
	s.url = u

	return nil
}

func (s *sock) connect(ctx context.Context) error {
	s.disconnect()

	s.infoLog <- "Opening socket..."

	// defined up here to make the redundant slice warning fuck off.
	var (
		ua      = []string{USER_AGENT}
		cookies = []string{s.cfg.Cookies}
	)

	// Create new WebSocket dialer, routing through any applicable proxies.
	wd := websocket.Dialer{
		EnableCompression: true,
		// Set handshake timeout to 5 mins.
		HandshakeTimeout: time.Minute * 5,
	}
	if s.proxy != nil {
		wd.NetDialContext = s.proxy.DialContext
	}

	conn, _, err := wd.DialContext(ctx, s.url.String(), map[string][]string{
		"Cookie":     cookies,
		"User-Agent": ua,
	})
	if err != nil {
		return err
	}
	conn.EnableWriteCompression(true)
	// Set s.Conn at the end to avoid early access.
	s.Conn = conn

	// Send /join message for desired room.
	s.Out <- fmt.Sprintf("/join %d", s.cfg.Room)
	s.infoLog <- "Connected."

	return nil
}

func (s *sock) disconnect() {
	if s.Conn != nil {
		s.Conn.Close()
		s.Conn = nil
	}
}

// Tries reconnecting 8 times.
func (s *sock) reconnect(ctx context.Context) error {
	for {
		for i := 0; i < 8; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			err := s.connect(ctx)
			if err == nil {
				return nil
			}
		}

		s.infoLog <- "Failed to connect 8 times. Waiting 15 seconds."
		time.Sleep(time.Second * 15)
	}
}

func (s *sock) write(msg string) (err error) {
	if s.Conn == nil {
		return &errSocketClosed{}
	}

	out := bytes.TrimSpace([]byte(msg))
	if len(out) == 0 {
		return errors.New("Outgoing msg is empty.")
	}

	return s.WriteMessage(websocket.TextMessage, out)
}

func (s *sock) msgReader(ctx context.Context) {
	host, err := url.Parse("https://" + s.url.Hostname())
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if s.Conn == nil {
			s.reconnect(ctx)
			continue
		}

		_, msg, err := s.ReadMessage()
		if err != nil {
			s.infoLog <- "Failed to read from socket.\n"
			s.reconnect(ctx)
			continue
		}

		// Server sometimes sends plaintext messages to client.
		// This typically happens when it sends error messages.
		switch ms := string(msg); {
		case json.Valid(msg):
			s.chatJson <- msg
		case strings.Contains(ms, "cannot join"):
			s.infoLog <- "Session expired. Refreshing token..."
			_, err := s.kf.RefreshSession(ctx)
			if err != nil {
				panic(err)
			}
			s.cfg.Cookies = s.kf.Client.Jar.(*libkiwi.KiwiJar).CookieString(host)
			s.connect(ctx)
		default:
			s.infoLog <- ms
		}

	}
}

func (s *sock) router(ctx context.Context) {
	// Join msg regex.
	joinRE := regexp.MustCompile(`^/join \d+`)

	cmdHandler := func(cs string) {
		switch cmd := strings.SplitN(cs, " ", 3); cmd[0] {
		case "!debug":
			// Various debugging tools.
			switch cmd[1] {
			case "msg":
				s.debug <- cmd[2]
			}
		case "!q", "!quit":
			return
		case "!reconnect":
			s.disconnect()
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-s.Out:
			isJoin := joinRE.MatchString(m)
			switch {
			case s.cfg.ReadOnly && !isJoin:
				continue
			case m[0] == '!':
				cmdHandler(m)
			default:
				if isJoin {
					room, err := strconv.Atoi(strings.Split(m, " ")[1])
					if err != nil {
						s.errLog <- err
					}
					s.cfg.Room = uint(room)
				}

				err := s.write(m)
				if err != nil {
					s.infoLog <- fmt.Sprintf("Failed to send: %s\nError: %s\n", m, err)
				}
			}
		}
	}
}

func (s *sock) Start(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		s.msgReader(ctx)
	}()
	go func() {
		defer wg.Done()
		s.router(ctx)
	}()
	stopf := context.AfterFunc(ctx, func() {
		defer wg.Done()
		s.Stop()
	})
	defer stopf()

	wg.Wait()
}

func (s *sock) Stop() {
	s.disconnect()
	if s.proxy != nil {
		s.proxy.stopTor()
	}
}
