package socket

import (
	"context"
	"log"
	"net"

	"github.com/cretz/bine/tor"
)

type ctx func(ctx context.Context, network string, addr string) (net.Conn, error)

type torInst struct {
	tor    *tor.Tor
	dialer *tor.Dialer
	torCtx ctx
}

func startTor() *tor.Tor {
	log.Print("Connecting to Tor network...")

	ti, err := tor.Start(nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	return ti
}

func (t *torInst) stopTor() *torInst {
	if t.tor == nil {
		return t
	}

	log.Print("Stopping Tor.")

	t.tor.Close()

	t.tor = nil
	t.dialer = nil
	t.torCtx = nil

	return t
}

func (t *torInst) getTorCtx() ctx {
	if t.tor == nil {
		t.tor = startTor()
	}

	td, err := t.tor.Dialer(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	t.dialer = td

	t.torCtx = td.DialContext
	return t.torCtx
}
