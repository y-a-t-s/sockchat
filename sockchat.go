package main

/*
 * SockChat
 *
 * @@@@@@@@@@@@@@,,.~`.@@@@@@@@@@@@
 * @@@@@@@@@@@@@,,,,,,^,,`.@@@@@@@@
 * @@@@@@@@@@$,,,,,,,,#@@@',,,,@@@@
 * @@@@@@@@@,,,,,,,,,,,,@@@@@@`,,@@
 * @@@@@@@?,,,,,,,,,,,,,,&@@@@@@@@@
 * @@@@@%,,,,,,,,^^,,,,,,,,@@@@@@@@
 * @@@@........%@@@@#........@@@@@@
 * @@@.......@@@@@@@@@?.......@@@@@
 * @@.......@@@@@@@@@@@@.......@@@@
 * @@......@@@@@@@@@@@@@@......@@@@
 * @@*****@@@@@@@@@@@@@@@@*****@@@@
 * @@@****@@@@@@@@@@@@@@@@****@@@@@
 * @@@@(***@@@@@@@@@@@@@@***(@@@@@@
 * @@@@@@@**%@@@@@@@@@&**@@@@@@@@@@
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

	"y-a-t-s/sockchat/chat"
	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/services"
)

func main() {
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

	// WaitGroup for main worker routines.
	// Ensures all routines terminate before the program exits.
	var wg sync.WaitGroup

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Panic(err)
	}
	// Preserve original cfg before overwriting values during arg parsing.
	// If we didn't, temporary args would be applied to the config on exit.
	args := cfg

	err = args.ParseArgs()
	if err != nil {
		log.Panic(err)
	}

	c, err := chat.NewChat(ctx, args)
	if err != nil {
		log.Panic(err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		c.Start(ctx)

		cfg.Cookies = c.Cfg.Cookies
		cfg.Save()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		services.StartTUI(ctx, c)
	}()
	wg.Wait()
	cancel()
}
