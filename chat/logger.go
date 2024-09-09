package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type logfile struct {
	file   *os.File
	writer *bufio.Writer
}

func openLog(name string) (lf logfile, err error) {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	bw := bufio.NewWriter(f)

	lf = logfile{
		file:   f,
		writer: bw,
	}
	return
}

func (lf *logfile) Close() {
	if lf.writer != nil {
		lf.writer.Flush()
		lf.writer = nil
	}

	if lf.file != nil {
		lf.file.Close()
		lf.file = nil
	}
}

type logger struct {
	chatLog logfile
	errLog  logfile

	feed chan *Message
	errs *slog.Logger

	logDir string
}

const dateFmt = "2006-01-02 15_04_05 MST"

func newLogDir(baseDir string) (string, error) {
	outDir := filepath.Join(baseDir, time.Now().Format("2006-01-02"))

	err := os.Mkdir(baseDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	err = os.Mkdir(outDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}

	return outDir, nil
}

func newLogger(chat bool) (l logger, err error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	baseDir := filepath.Join(cfgDir, "sockchat/logs")

	logDir, err := newLogDir(baseDir)
	if err != nil {
		return
	}
	l.logDir = logDir

	if chat {
		l.chatLog, err = l.newChatLog()
		if err != nil {
			return
		}
	}

	elFile, err := l.newErrLog()
	if err != nil {
		return
	}
	elw := bufio.NewWriter(elFile.file)
	l.errs = slog.New(slog.NewTextHandler(elw, nil))

	l.feed = l.newMsgFeed()

	return
}

func (l *logger) newChatLog() (lf logfile, err error) {
	// See time.Format docs to make sense of the date string.
	// Note: Windows doesn't allow ':' in file names. Use '_' instead.

	if l.feed == nil {
		l.feed = l.newMsgFeed()
	}

	return openLog(filepath.Join(l.logDir, fmt.Sprintf("%s.log", time.Now().Format(dateFmt))))
}

func (l *logger) newErrLog() (lf logfile, err error) {
	return openLog(filepath.Join(l.logDir, fmt.Sprintf("%s error.log", time.Now().Format(dateFmt))))
}

func (l *logger) newMsgFeed() chan *Message {
	feed := make(chan *Message, 1024)
	l.feed = feed
	return feed
}

func (l *logger) Start(ctx context.Context) {
	defer func() {
		l.Stop()
	}()

	const logFmt = "[%s] [%s (#%d)]%s: %s\n"
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-l.feed:
			if l.chatLog.writer == nil {
				continue
			}

			fl := ""
			if msg.IsEdited() {
				fl += "*"
			}

			fmt.Fprintf(l.chatLog.writer, logFmt, time.Unix(msg.MessageDate, 0).Format("2006-01-02 15:04:05 MST"),
				msg.Author.Username, msg.Author.ID, fl, msg.MessageRaw)
		}
	}
}

func (l *logger) Stop() {
	l.chatLog.Close()
	l.errLog.Close()
}
