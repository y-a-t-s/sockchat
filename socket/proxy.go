package socket

import (
	"context"
	"net"

	"golang.org/x/net/proxy"
)

type proxyCtx func(ctx context.Context, network string, addr string) (net.Conn, error)

func newSocksDialer(addr string, auth *proxy.Auth) (proxy.Dialer, error) {
	d, err := proxy.SOCKS5("tcp", addr, auth, &net.Dialer{})
	if err != nil {
		return d, err
	}

	return d, nil
}
