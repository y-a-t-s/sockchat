package socket

import (
	"context"
	"errors"
	"log"

	"github.com/cretz/bine/tor"
)

type torConn struct {
	*tor.Tor
	proxy       proxyCtx
	proxyDialer *tor.Dialer
}

func (t *torConn) startTor(ctx context.Context) error {
	log.Println("Connecting to Tor network...")

	ti, err := tor.Start(ctx, nil)
	if err != nil {
		return err
	}
	t.Tor = ti
	t.newTorProxyCtx(ctx)

	return nil
}

func (t *torConn) stopTor() {
	if t.Tor == nil {
		return
	}

	log.Println("Stopping Tor.")

	t.Close()

	t.Tor = nil
	t.proxyDialer = nil
	t.proxy = nil
}

func (t *torConn) newTorProxyCtx(ctx context.Context) (proxyCtx, error) {
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
