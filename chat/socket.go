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
	// Color used for greentext.
	// #72ff72 is used by Dark Reader and works nicely against the web background.
	GREEN = "#72ff72"
	// Max chat history length.
	HIST_LEN = 512
	// User-Agent string for headers.
	USER_AGENT = "Mozilla/5.0 (Windows NT 6.1; rv:60.0) Gecko/20100101 Firefox/60.0"
)

type sock struct {
	*websocket.Conn
	closed chan struct{}

	Users *userTable
	pool  *ChatPool

	debug   chan string
	errLog  chan error
	infoLog chan string

	chatJson chan []byte
	messages chan *Message
	Out      chan string

	proxy *socksProxy
	host  *url.URL

	Cfg config.Config
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
		Cfg:    cfg,
		closed: make(chan struct{}),

		Users: NewUserTable(uint32(cfg.UserID)),
		pool:  newChatPool(),

		debug:    make(chan string, 8),
		errLog:   make(chan error, 8),
		infoLog:  make(chan string, 8),
		chatJson: make(chan []byte, 64),
		messages: make(chan *Message, HIST_LEN),
		Out:      make(chan string, 8),
	}
	close(s.closed)

	err := s.setUrl(cfg.Host, uint16(cfg.Port))
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(s.host.Hostname(), ".onion") {
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

			log.Printf("Connecting to onion service: %s\nMake sure this domain is correct.\n", s.host.Hostname())
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

	kf, err := libkiwi.NewKF(hc, s.host.Hostname(), cfg.Cookies)
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
	s.host = u

	return nil
}

func (s *sock) connect(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
	default:
		s.disconnect()
		// return errors.New("Socket already open.")
	}

	s.infoLog <- "Opening socket..."

	// defined up here to make the redundant slice warning fuck off.
	var (
		ua = []string{USER_AGENT}
		//cookies = []string{s.Cfg.Cookies}
	)

	// Create new WebSocket dialer, routing through any applicable proxies.
	wd := websocket.Dialer{
		EnableCompression: true,
		// Set handshake timeout to 1 min.
		HandshakeTimeout: time.Minute,
		Jar:              s.kf.Client.Jar,
	}
	if s.proxy != nil {
		wd.NetDialContext = s.proxy.DialContext
	}

	conn, _, err := wd.DialContext(ctx, s.host.String(), map[string][]string{
		//"Cookie":     cookies,
		"User-Agent": ua,
	})
	if err != nil {
		return err
	}
	conn.EnableWriteCompression(true)
	// Set s.Conn at the end to avoid early access.
	s.Conn = conn

	// Mark as opened.
	s.closed = make(chan struct{})

	// Send /join message for desired room.
	s.Out <- fmt.Sprintf("/join %d", s.Cfg.Room)
	s.infoLog <- "Connected."

	return nil
}

func (s *sock) disconnect() {
	select {
	case <-s.closed:
	default:
		close(s.closed)
		if s.Conn == nil {
			s.Conn.Close()
			s.Conn = nil
		}
	}
}

// Tries reconnecting 8 times.
func (s *sock) reconnect(ctx context.Context) {
	for {
		for i := 0; i < 8; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := s.connect(ctx)
			if err == nil {
				return
			}

			s.errLog <- err
			time.Sleep(time.Second * 5)
		}

		s.infoLog <- "Failed to connect 8 times. Waiting 1 minute."
		time.Sleep(time.Minute)
	}
}

func (s *sock) read() ([]byte, error) {
	select {
	case <-s.closed:
		return nil, &errSocketClosed{}
	default:
	}

	_, msg, err := s.ReadMessage()
	if err != nil {
		s.infoLog <- "Failed to read from socket.\n"
		return nil, err
		// s.reconnect(ctx)
	}

	return msg, nil
}

func (s *sock) write(msg string) error {
	out := bytes.TrimSpace([]byte(msg))
	if len(out) == 0 {
		return errors.New("Outgoing msg is empty.")
	}

	select {
	case <-s.closed:
		return &errSocketClosed{}
	default:
		return s.WriteMessage(websocket.TextMessage, out)
	}
}

func (s *sock) msgReader(ctx context.Context) {
	// Flag for session cookie refresh attempt.
	var refreshed bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closed:
			s.reconnect(ctx)
		default:
		}

		msg, err := s.read()
		if err != nil {
			s.errLog <- err
			s.reconnect(ctx)

			continue
		}

		// Server sometimes sends plaintext messages to client.
		// This typically happens when it sends error messages.
		switch ms := string(msg); {
		case json.Valid(msg):
			// Reset cookie refresh flag if chat messages were read successfully.
			refreshed = false
			go s.ParseResponse(ctx, msg)
		case strings.Contains(ms, "cannot join"):
			if refreshed {
				s.infoLog <- "Unable to join chat. Cookies possibly expired. Try providing new ones."
				// Wait until context close (quit).
				<-ctx.Done()
			}

			s.infoLog <- "Session expired. Refreshing token..."
			refreshed = true

			_, err := s.kf.RefreshSession(ctx)
			if err != nil {
				s.errLog <- err
				continue
			}
			s.Cfg.Cookies = s.kf.Client.Jar.(*libkiwi.KiwiJar).CookieString(s.host)

			s.reconnect(ctx)
		default:
			s.infoLog <- ms
		}

	}
}

func (s *sock) router(ctx context.Context) {
	// Join msg regex.
	joinRE := regexp.MustCompile(`^/join \d+`)
	greenRE := regexp.MustCompile(`^>\w`)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-s.Out:
			isJoin := joinRE.MatchString(msg)
			if msg == "" || (s.Cfg.ReadOnly && !isJoin) {
				continue
			}

			switch {
			case isJoin:
				room, err := strconv.Atoi(strings.Split(msg, " ")[1])
				if err != nil {
					s.errLog <- err
					continue
				}
				s.Cfg.Room = uint(room)
			case greenRE.MatchString(msg):
				msg = fmt.Sprintf("[color=%s]%s", GREEN, msg)
			}

			err := s.write(msg)
			if err != nil {
				s.infoLog <- fmt.Sprintf("Failed to send: %s\n", msg)
				s.errLog <- err

				continue // To help prevent future fuckups.
			}
		}
	}
}

func (s *sock) start(ctx context.Context) {
	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel()
		s.msgReader(ctx)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		s.router(ctx)
		s.stop()
	}()
	wg.Wait()
}

func (s *sock) stop() {
	s.disconnect()
	if s.proxy != nil {
		s.proxy.stopTor()
	}
}
