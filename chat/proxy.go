package chat

import (
	"context"
	"log"
	"net"
	"net/url"
	"strings"
	"y-a-t-s/sockchat/config"

	"github.com/cretz/bine/tor"
	"golang.org/x/net/proxy"
)

type socksProxy struct {
	proxy.ContextDialer

	url *url.URL
	tor *tor.Tor
}

// user and pass may be left empty if credentials are supplied in addr.
func parseProxyAddr(addr string, user string, pass string) (*url.URL, error) {
	// Fallback to socks5 if no protocol is given.
	if !strings.Contains(addr, "://") {
		addr = "socks5://" + addr
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	// url.Parse collects any credentials in the URL to a *url.Userinfo.
	// If none are found, the pointer is nil.
	// Credentials in the URL take precedence over explicit ones in the config.
	if u.User == nil && user != "" {
		// Create new &url.Userinfo with the explicit credentials.
		u.User = url.UserPassword(user, pass)
	}

	return u, nil
}

func newSocksDialer(cfg config.Config) (p *socksProxy, err error) {
	u, err := parseProxyAddr(cfg.Proxy.Addr, cfg.Proxy.User, cfg.Proxy.Pass)
	if err != nil {
		return
	}

	d, err := proxy.FromURL(u, &net.Dialer{})
	if err != nil {
		return
	}

	p = &socksProxy{
		ContextDialer: d.(proxy.ContextDialer),
		url:           u,
	}
	return
}

func startTor(ctx context.Context) (p *socksProxy, err error) {
	log.Println("Connecting to Tor network...")
	ti, err := tor.Start(ctx, nil)
	if err != nil {
		return
	}

	td, err := ti.Dialer(ctx, nil)
	if err != nil {
		return
	}

	p = &socksProxy{
		ContextDialer: td.Dialer.(proxy.ContextDialer),
		tor:           ti,
	}
	return
}

func (p *socksProxy) stopTor() {
	if p.tor == nil {
		return
	}

	log.Println("Stopping Tor.")

	p.tor.Close()
	p.tor = nil
}
