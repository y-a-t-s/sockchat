package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

type Logger interface {
	Log(entry string) error
	Start(ctx context.Context)
}

type logger struct {
	*bufio.Writer
	feed chan string
}

func NewLogger(ctx context.Context) (Logger, error) {
	const logDir = "logs"
	t := time.Now()
	outDir := fmt.Sprintf("%s/%s", logDir, t.Format("2006-01-02"))

	err := os.Mkdir(logDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	err = os.Mkdir(outDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}

	// See time.Format docs to make sense of the date string.
	fname := fmt.Sprintf("%s/%s.log", outDir, t.Format("2006-01-02 15:04:05 MST"))
	logFile, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return &logger{
		bufio.NewWriter(logFile),
		make(chan string, 1024),
	}, nil
}

func (l *logger) Log(entry string) error {
	if l.Writer == nil {
		return errors.New("Log writer is closed.")
	}

	l.feed <- entry
	return nil
}

func (l *logger) Start(ctx context.Context) {
	defer l.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-l.feed:
			l.WriteString(msg)
		}
	}
}
