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
	InputBox *tview.InputField
}

func InitUI(sock *socket.ChatSocket) *UI {
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
	chatView.SetBorder(false)

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
			sock.TryClose()
			app.Stop()
		}
	})
	msgBox.SetLabel("Message: ")

	flex.AddItem(chatView, 0, 1, false)
	flex.AddItem(msgBox, 1, 1, false)

	app.SetRoot(flex, true).SetFocus(msgBox)

	ui := &UI{app, flex, chatView, msgBox}
	go ui.incomingHandler(sock)

	return ui
}

func tabHandler(sock *socket.ChatSocket, msg string) string {
	return regexp.MustCompile(`@(\d+)`).ReplaceAllStringFunc(msg, func(m string) string {
		id := m[1:]
		if u := sock.GetUsername(id); u != id {
			return fmt.Sprintf("@%s,", u)
		}

		return m
	})
}

func (ui *UI) incomingHandler(sock *socket.ChatSocket) {
	tagRE := regexp.MustCompile(`\[(.+?)\]`)
	escapeTags := func(msg string) string {
		return tagRE.ReplaceAllString(msg, "[$1[]")
	}

	var prev *socket.ChatMessage
	for {
		msg := <-sock.Channels.Messages
		if prev == nil || msg != *prev {
			unEsc := escapeTags(html.UnescapeString(msg.MessageRaw))

			h, m, s := time.Unix(msg.MessageDate, 0).Clock()
			// Print timestamp with user's color.
			fmt.Fprintf(ui.ChatView, "[%s::u]%0.2d:%0.2d:%0.2d[-::U] ", msg.Author.GetColor(), h, m, s)

			// Print chat message, preceded by the sender's username and ID.
			fmt.Fprintf(ui.ChatView, "[\"%d\"]%s %s[\"\"]\n", msg.Author.ID, msg.Author.GetUserString(), unEsc)

			ui.ChatView.ScrollToEnd()
		}

		prev = &msg
	}
}
