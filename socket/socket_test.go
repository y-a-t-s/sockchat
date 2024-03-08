package socket

import (
	"testing"

	"y-a-t-s/sockchat/config"
)

func TestSocket(t *testing.T) {
	cfg, err := config.ParseArgs()
	if err != nil {
		t.Error(err)
	}

	sock, err := NewSocket(cfg)
	if err != nil {
		t.Error(err)
	}
	if _, err := sock.connect(); err != nil {
		t.Error(err)
	}

	sock.stopTor()
	sock.CloseAll()
}
