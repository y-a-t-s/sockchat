package chat

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const _DATE_FMT = "2006-01-02 15_04_05 MST"
const _LOG_FMT = "[%s] [%s (#%d)]%s: %s\n"

func openLog(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

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

func startLogger(in <-chan Message) error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	baseDir := filepath.Join(cfgDir, "sockchat/logs")
	logDir, err := newLogDir(baseDir)
	if err != nil {
		return err
	}

	lf, err := openLog(filepath.Join(logDir, fmt.Sprintf("%s.log", time.Now().Format(_DATE_FMT))))
	if err != nil {
		return err
	}
	lw := bufio.NewWriter(lf)

	go func() {
		defer func() {
			lw.Flush()
			lf.Close()
		}()

		for msg := range in {
			fl := ""
			if msg.IsEdited() {
				fl += "*"
			}

			fmt.Fprintf(lw, _LOG_FMT, time.Unix(msg.MessageDate, 0).Format("2006-01-02 15:04:05 MST"),
				msg.Author.Username, msg.Author.ID, fl, msg.MessageRaw)

			msg.Release()
		}
	}()

	return nil
}
