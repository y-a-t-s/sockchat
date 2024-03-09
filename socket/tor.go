package socket

import (
	"context"
	"errors"
	"log"
	"net"

	"github.com/cretz/bine/tor"
)

type torProxy func(ctx context.Context, network string, addr string) (net.Conn, error)

type torInst struct {
	*tor.Tor
	proxy       torProxy
	proxyDialer *tor.Dialer
}

func (t *torInst) startTor(ctx context.Context) error {
	log.Println("Connecting to Tor network...")

	ti, err := tor.Start(ctx, nil)
	if err != nil {
		return err
	}
	t.Tor = ti

	td, err := t.Dialer(ctx, nil)
	if err != nil {
		return err
	}
	t.proxyDialer = td
	t.proxy = td.DialContext

	return nil
}

func (t *torInst) stopTor() {
	if t.Tor == nil {
		return
	}

	log.Println("Stopping Tor.")

	t.Close()

	t.Tor = nil
	t.proxyDialer = nil
	t.proxy = nil
}

func (t *torInst) getTorProxy(ctx context.Context) (torProxy, error) {
	if t.Tor == nil {
		return nil, errors.New("Not connected to Tor network.")
	}

	td, err := t.Dialer(ctx, nil)
	if err != nil {
		return nil, err
	}
	t.proxyDialer = td

	t.proxy = td.DialContext
	return t.proxy, nil
}
