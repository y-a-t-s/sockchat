package tui

import (
	"bufio"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"time"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/socket"

	"github.com/gdamore/tcell/v2"
	"github.com/gen2brain/beeep"
	"github.com/rivo/tview"
)

type ui struct {
	App      *tview.Application
	MainView *tview.Flex
	ChatView *tview.TextView
	InputBox *tview.InputField

	logWriter *bufio.Writer
}

func InitUI(c socket.Socket, cfg config.Config) {
	app := tview.NewApplication()
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	chatView := tview.NewTextView().
		SetDynamicColors(true).
		SetMaxLines(2048).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	msgBox := tview.NewInputField().
		SetAcceptanceFunc(tview.InputFieldMaxLength(1024)).
		SetFieldBackgroundColor(tcell.PaletteColor(0)).
		SetFieldWidth(0).
		SetLabel("> ")

	ui := ui{app, flex, chatView, msgBox, nil}

	chatView.SetBorder(false)
	chatView.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyBacktab:
			flex.AddItem(msgBox, 1, 1, true)
			app.SetFocus(flex)
		case tcell.KeyCtrlC:
			ui.cleanup()
		}
	})

	msgBox.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			msg := strings.TrimSpace(msgBox.GetText())
			// Add outgoing message to queue.
			c.Send(msg)
			msgBox.SetText("")
		case tcell.KeyTab:
			msg := msgBox.GetText()
			msgBox.SetText(tabHandler(c, msg))
		case tcell.KeyBacktab:
			flex.RemoveItem(msgBox)
			app.SetFocus(chatView)
		case tcell.KeyCtrlC:
			ui.cleanup()
		}
	})

	flex.AddItem(chatView, 0, 1, false)
	if !cfg.ReadOnly {
		flex.AddItem(msgBox, 1, 1, true)
	}

	app.SetRoot(flex, true).SetFocus(flex)

	go ui.incomingHandler(c)
	app.Run()
	ui.cleanup()
}

func tabHandler(c socket.Socket, msg string) string {
	return regexp.MustCompile(`@(\d+)`).ReplaceAllStringFunc(msg, func(m string) string {
		id := m[1:]
		if u := c.QueryUser(id); u != id {
			return fmt.Sprintf("@%s,", u)
		}

		return m
	})
}

func (u *ui) cleanup() {
	if u.logWriter != nil {
		u.logWriter.Flush()
	}
	u.App.Stop()
}

func (u *ui) logger(logFeed chan string) error {
	const logDir = "logs"
	t := time.Now()
	outDir := fmt.Sprintf("%s/%s", logDir, t.Format("2006-01-02"))

	err := os.Mkdir(logDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	err = os.Mkdir(outDir, 0755)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	fname := fmt.Sprintf("%s/%s.log", outDir, t.Format("2006-01-02 15:04:05 MST"))
	logFile, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	u.logWriter = bufio.NewWriter(logFile)
	defer u.logWriter.Flush()

	for {
		select {
		case msg := <-logFeed:
			u.logWriter.WriteString(msg)
		}
	}
}

func (u *ui) incomingHandler(c socket.Socket) {
	tagRE := regexp.MustCompile(`\[(.+?)\]`)
	escapeTags := func(msg string) string {
		return tagRE.ReplaceAllString(msg, "[$1[]")
	}

	var mentionRE *regexp.Regexp
	// IDs of any messages that mention the user. Used for message highlighting.
	var mentionIDs []string
	mentionHandler := func(msg *socket.ChatMessage) {
		if mentionRE == nil {
			cn := c.GetClientName()
			if len(cn) == 0 {
				return
			}
			mentionRE = regexp.MustCompile(fmt.Sprintf("@%s,", cn))
		}

		if mentionRE.MatchString(msg.MessageRaw) {
			beeep.Notify("New mention", html.UnescapeString(msg.MessageRaw), "")
			mentionIDs = append(mentionIDs, fmt.Sprint(msg.MessageID))
			u.ChatView.Highlight(mentionIDs...)
		}
	}

	logFeed := make(chan string, 128)
	go u.logger(logFeed)

	var prev *socket.ChatMessage
	for {
		msg := c.GetIncoming()
		if prev == nil || msg != *prev {
			unEsc := escapeTags(html.UnescapeString(msg.MessageRaw))

			h, m, s := time.Unix(msg.MessageDate, 0).Clock()
			// Format timestamp with user's color.
			timestamp := fmt.Sprintf("%0.2d:%0.2d:%0.2d", h, m, s)

			logFeed <- fmt.Sprintf("[%s] [%s (#%d)]: %s\n", timestamp,
				msg.Author.Username, msg.Author.ID, unEsc)

			// Print chat message, preceded by the sender's username and ID.
			fmt.Fprintf(u.ChatView, "[%s::u]%s[-::U] %s [\"%d\"]%s[\"\"]\n",
				msg.Author.GetColor(), timestamp, msg.Author.GetUserString(),
				msg.MessageID, unEsc)
			u.ChatView.ScrollToEnd()
			mentionHandler(&msg)
		}

		prev = &msg
	}
}
