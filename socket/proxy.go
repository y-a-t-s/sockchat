package socket

import (
	"context"
	"log"
	"net"
	"net/url"

	"golang.org/x/net/proxy"

	"github.com/cretz/bine/tor"
)

type socksProxy struct {
	proxy.Dialer
	dialCtx proxyCtx

	url *url.URL
	tor *tor.Tor
}

type proxyCtx func(ctx context.Context, network string, addr string) (net.Conn, error)

func newSocksDialer(addr url.URL) (socksProxy, error) {
	var p socksProxy

	d, err := proxy.FromURL(&addr, &net.Dialer{})
	if err != nil {
		return p, err
	}

	p.Dialer = d
	p.dialCtx = d.(proxy.ContextDialer).DialContext
	p.url = &addr
	return p, nil
}

func startTor(ctx context.Context) (socksProxy, error) {
	var p socksProxy

	log.Println("Connecting to Tor network...")
	ti, err := tor.Start(ctx, nil)
	if err != nil {
		return p, err
	}

	td, err := ti.Dialer(ctx, nil)
	if err != nil {
		return p, err
	}

	p.Dialer = td.Dialer
	p.dialCtx = td.DialContext
	p.tor = ti
	return p, nil
}

func (p *socksProxy) stopTor() {
	if p.tor == nil {
		return
	}

	log.Println("Stopping Tor.")

	p.tor.Close()
	p.tor = nil
}
