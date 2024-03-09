package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/services"
	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

func main() {
	cfg, err := config.ParseArgs()
	if err != nil {
		log.Panic(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGABRT, syscall.SIGHUP,
		syscall.SIGINT, syscall.SIGKILL,
		syscall.SIGPIPE, syscall.SIGQUIT,
		syscall.SIGSEGV, syscall.SIGSTOP,
		syscall.SIGTERM)
	defer cancel()

	sock, err := socket.NewSocket(ctx, cfg)
	if err != nil {
		log.Panic(err)
	}

	var wg sync.WaitGroup

	var l services.Logger
	if cfg.Logger {
		l, err = services.NewLogger(ctx)
		if err != nil {
			log.Panic(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			l.Start(ctx)
		}()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel()
		sock.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		tui.InitUI(ctx, sock, cfg, l)
	}()
	wg.Wait()
	cancel()
}
