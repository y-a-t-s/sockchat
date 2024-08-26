package chat

import (
	"context"
	"log"
	"net"
	"net/url"

	"github.com/cretz/bine/tor"
	"golang.org/x/net/proxy"
)

type socksProxy struct {
	proxy.ContextDialer

	url *url.URL
	tor *tor.Tor
}

func newSocksDialer(addr *url.URL) (p *socksProxy, err error) {
	d, err := proxy.FromURL(addr, &net.Dialer{})
	if err != nil {
		return
	}

	p = &socksProxy{
		ContextDialer: d.(proxy.ContextDialer),
		url:           addr,
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
