package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type logger struct {
	*bufio.Writer
	feed chan *ChatMessage
}

func (l *logger) Start(ctx context.Context) error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(cfgDir, "sockchat/logs")

	t := time.Now()
	outDir := filepath.Join(logDir, t.Format("2006-01-02"))

	err = os.Mkdir(logDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	err = os.Mkdir(outDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	dateFmt := "2006-01-02 15:04:05 MST"
	// See time.Format docs to make sense of the date string.
	if runtime.GOOS == "windows" {
		dateFmt = "2006-01-02 15_04_05 MST"
	}
	logPath := filepath.Join(outDir, fmt.Sprintf("%s.log", t.Format(dateFmt)))
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.Writer = bufio.NewWriter(logFile)
	l.feed = make(chan *ChatMessage, 1024)

	defer func() {
		l.Flush()
		logFile.Close()
		l.feed = nil
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	context.AfterFunc(ctx, func() {
		close(l.feed)
	})

	logFmt := "[%s] [%s (#%d)]%s: %s\n"
	for msg := range l.feed {
		if l.Writer == nil {
			return errors.New("Log writer is closed.")
		}

		fl := ""
		if msg.IsEdited() {
			fl += "*"
		}

		fmt.Fprintf(l, logFmt, time.Unix(msg.MessageDate, 0).Format("2006-01-02 15:04:05 MST"),
			msg.Author.Username, msg.Author.ID, fl, msg.MessageRaw)
	}

	return nil
}
