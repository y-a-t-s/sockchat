package tui

import (
	"fmt"
	"regexp"
	"strings"

	"y-a-t-s/sockchat/chat"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"nhooyr.io/websocket"
)

type UI struct {
	App      *tview.Application
	MainView *tview.Flex
	ChatView *tview.TextView
	InputBox *tview.InputField
}

func (ui *UI) ChatHandler(sock *chat.ChatSocket, prev *chat.ChatMessage) *UI {
	select {
	case msg := <-sock.Received:
		if prev == nil || msg != *prev {
			fmt.Fprintf(ui.ChatView, "%s (#%d): %s\n", msg.Author.Username, msg.Author.ID, msg.MessageRaw)
			ui.ChatView.ScrollToEnd()
		}
		return ui.ChatHandler(sock, &msg)
	}
}

func NewUI(sock *chat.ChatSocket) *UI {
	app := tview.NewApplication()
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	chatView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			app.Draw()
		}).
		SetDoneFunc(func(key tcell.Key) {
			switch key {
			case tcell.KeyCtrlC:
				app.Stop()
				return
			}
		})
	chatView.SetBorder(false)

	userRe := regexp.MustCompile(`@(\d+)`)
	msgBox := tview.NewInputField().
		SetFieldWidth(0).
		SetAcceptanceFunc(tview.InputFieldMaxLength(1024))
	msgBox.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			msg := msgBox.GetText()
			strings.TrimSpace(msg)
			if len(msg) == 0 {
				return
			}

			err := sock.Conn.Write(sock.Context, websocket.MessageText, []byte(msg))
			if err != nil {
				sock.ClientMsg("Failed to send msg.")
			}
			msgBox.SetText("")
		case tcell.KeyTab:
			msgBox.SetText(string(
				userRe.ReplaceAllFunc([]byte(msgBox.GetText()), func(m []byte) []byte {
					id := string(m[1:])
					if u, exists := sock.Users[id]; exists {
						return []byte(fmt.Sprintf("@%s,", u))
					}

					return m
				})))
		case tcell.KeyCtrlC:
			sock.Conn.CloseNow()
			app.Stop()
		}
	})
	msgBox.SetLabel("Message: ")

	flex.AddItem(chatView, 0, 1, false)
	flex.AddItem(msgBox, 1, 1, false)

	app.SetRoot(flex, true).SetFocus(msgBox)
	return &UI{app, flex, chatView, msgBox}
}
