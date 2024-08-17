package main

/*
 * SockChat
 *
 * @@@@@@@@@@@@,,.~`.@@@@@@@@@@@@
 * @@@@@@@@@@@,,,,,,^,,`.@@@@@@@@
 * @@@@@@@@$,,,,,,,,#@@@',,,,@@@@
 * @@@@@@@,,,,,,,,,,,,@@@@@@`,,@@
 * @@@@@?,,,,,,,,,,,,,,&@@@@@@@@@
 * @@@%,,,,,,,,^^,,,,,,,,@@@@@@@@
 * @@........%@@@@#........@@@@@@
 * @.......@@@@@@@@@?.......@@@@@
 * .......@@@@@@@@@@@@.......@@@@
 * ......@@@@@@@@@@@@@@......@@@@
 * *****@@@@@@@@@@@@@@@@*****@@@@
 * @****@@@@@@@@@@@@@@@@****@@@@@
 * @@(***@@@@@@@@@@@@@@***(@@@@@@
 * @@@@@**%@@@@@@@@@&**@@@@@@@@@@
 *
 * Logo credit: DPS (#153597)
 */

// Made with <3 for the KF community.

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
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Panic(err)
	}
	if err := cfg.ParseArgs(); err != nil {
		log.Panic(err)
	}

	// Catch terminating signals to try shutting down gracefully.
	// Needed to ensure log buffer gets flushed.
	sigs := []os.Signal{
		syscall.SIGABRT, syscall.SIGHUP,
		syscall.SIGINT, syscall.SIGKILL,
		syscall.SIGPIPE, syscall.SIGQUIT,
		syscall.SIGSEGV, syscall.SIGTERM,
	}
	if runtime.GOOS != "windows" {
		// syscall.SIGSTOP (0x13) is not defined when targeting Windows.
		var stop syscall.Signal = 0x13
		sigs = append(sigs, stop)
	}
	// Create context that cancels on detection of any signals listed above.
	ctx, cancel := signal.NotifyContext(context.Background(), sigs...)
	defer cancel()

	sock, err := socket.NewSocket(ctx, cfg)
	if err != nil {
		log.Panic(err)
	}
	context.AfterFunc(ctx, sock.Stop)

	// WaitGroup for main worker routines.
	// Ensures all routines terminate before the program exits.
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
