package main

/*
 * SockChat
 *
 * @@@@@@@@@@@@@/,,,,,@@@@@@@@@@@
 * @@@@@@@@@%,,,,,,,,@@@(,,,,(@@@
 * @@@@@@@(,,,,,,,,,,#@@@@@@@@*,#
 * @@@@@*,,,,,,,,,,,,,,/@@@@@@@@@
 * @@@@,,,,,,,,(@,,,,,,,,@@@@@@@@
 * @@,,,,,,,,%@@@@#,,,,,,,,@@@@@@
 * @,,,,,,,@@@@@@@@@,,,,,,,,@@@@@
 * ,,,,,,,@@@@@@@@@@@@,,,,,,*@@@@
 * ,,,,,,@@@@@@@@@@@@@@,,,,,*@@@@
 * *****@@@@@@@@@@@@@@@@*****@@@@
 * @****@@@@@@@@@@@@@@@@****@@@@@
 * @@(***@@@@@@@@@@@@@@***(@@@@@@
 * @@@@@**%@@@@@@@@@&**@@@@@@@@@@
 *
 * By: The KF community
 * Logo credit: DPS
 */

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/services"
	"y-a-t-s/sockchat/socket"
)

func main() {
	cfg, err := config.ParseArgs()
	if err != nil {
		log.Panic(err)
	}

	sigs := []os.Signal{
		syscall.SIGABRT, syscall.SIGHUP,
		syscall.SIGINT, syscall.SIGKILL,
		syscall.SIGPIPE, syscall.SIGQUIT,
		syscall.SIGSEGV, syscall.SIGTERM,
	}
	if runtime.GOOS != "windows" {
		// syscall.SIGSTOP is not defined when targeting Windows.
		var stop syscall.Signal = 0x13
		sigs = append(sigs, stop)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), sigs...)
	defer cancel()

	sock, err := socket.NewSocket(ctx, cfg)
	if err != nil {
		log.Panic(err)
	}
	context.AfterFunc(ctx, sock.CloseAll)

	var wg sync.WaitGroup

	var l services.Logger
	if cfg.Logger {
		l, err = services.NewLogger()
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
		services.InitUI(ctx, sock, cfg, l)
	}()
	wg.Wait()
	cancel()
}
