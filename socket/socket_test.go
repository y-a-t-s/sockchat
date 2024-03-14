package socket

import (
	"context"
	"testing"
	"time"

	"y-a-t-s/sockchat/config"
)

func TestSocket(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Error(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := NewSocket(ctx, cfg)
	if err != nil {
		t.Error(err)
	}
	if _, err := s.(*sock).connect(ctx); err != nil {
		t.Error(err)
	}

	s.CloseAll()
	cancel()
}
