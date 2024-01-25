package tui

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"y-a-t-s/sockchat/socket"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type UI struct {
	App      *tview.Application
	MainView *tview.Flex
	ChatView *tview.TextView
	TimeBox  *tview.TextView
	InputBox *tview.InputField
}

func tabHandler(sock *socket.ChatSocket, msg string) string {
	return regexp.MustCompile(`@(\d+)`).ReplaceAllStringFunc(msg, func(m string) string {
		id := m[1:]

		if u := sock.GetUsername(id); u != id {
			return fmt.Sprintf("@%s,", u)
		}

		return fmt.Sprintf("@%s", id)
	})
}

func (ui *UI) printEmptyLine() {
	fmt.Fprintln(ui.ChatView)
	fmt.Fprintln(ui.TimeBox)

	ui.ChatView.ScrollToEnd()
	ui.TimeBox.ScrollToEnd()
}

func (ui *UI) incomingHandler(sock *socket.ChatSocket) {
	var prev *socket.ChatMessage
	for {
		msg := <-sock.Channels.Messages
		if prev == nil || msg != *prev {
			unEsc := escapeTags(html.UnescapeString(msg.MessageRaw))
			nnl := strings.TrimSuffix(unEsc, "\n")
			fmt.Fprintf(ui.ChatView, "%s %s\n", msg.Author.GetUserString(), nnl)
			h, m, s := func() (int, int, int) {
				if msg.Author.ID == 0 {
					return time.Now().Clock()

				}
				return time.Unix(int64(msg.MessageDate), 0).Clock()
			}()
			fmt.Fprintf(ui.TimeBox, "[%s::u]%0.2d:%0.2d:%0.2d[-::U]\n", msg.Author.GetColor(), h, m, s)

			if len(nnl) < len(unEsc) {
				ui.printEmptyLine()
			}

			ui.ChatView.ScrollToEnd()
			ui.TimeBox.ScrollToEnd()
		}

		prev = &msg
	}
}

func escapeTags(msg string) string {
	return regexp.MustCompile(`\[(.+?)\]`).ReplaceAllString(msg, "[$1[]")
}

func InitUI(sock *socket.ChatSocket) *UI {
	app := tview.NewApplication()
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	chatView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	chatView.SetBorder(false)

	inner := tview.NewFlex().SetDirection(tview.FlexColumn)

	timeBox := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
		})
	timeBox.SetBorder(false)

	msgBox := tview.NewInputField().
		SetFieldWidth(0).
		SetAcceptanceFunc(tview.InputFieldMaxLength(1024))
	msgBox.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			msg := strings.TrimSpace(msgBox.GetText())

			if !sock.ChatDebug(msg) {
				// Add outgoing message to queue
				sock.Channels.Outgoing <- []byte(msg)
			}
			msgBox.SetText("")
		case tcell.KeyTab:
			msg := msgBox.GetText()
			msgBox.SetText(tabHandler(sock, msg))
		case tcell.KeyCtrlC:
			sock.Conn.Close()
			app.Stop()
		}
	})
	msgBox.SetLabel("Message: ")

	inner.AddItem(chatView, 0, 1, false)
	inner.AddItem(timeBox, 8, 1, false)

	flex.AddItem(inner, 0, 1, false)
	flex.AddItem(msgBox, 1, 1, false)

	app.SetRoot(flex, true).SetFocus(msgBox)

	ui := &UI{app, flex, chatView, timeBox, msgBox}
	go ui.incomingHandler(sock)

	return ui
}
