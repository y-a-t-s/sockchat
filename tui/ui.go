package tui

import (
	"fmt"
	"regexp"
	"strings"

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

func acHandler(sock *socket.ChatSocket, msg string, re *regexp.Regexp) string {
	return string(re.ReplaceAllFunc([]byte(msg), func(m []byte) []byte {
		id := string(m[1:])

		sock.Users.UsersMutex.Lock()
		if u, exists := sock.Users.UserMap[id]; exists {
			m = []byte(fmt.Sprintf("@%s,", u))
		}
		sock.Users.UsersMutex.Unlock()

		return m
	}))
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

	acRE := regexp.MustCompile(`@(\d+)`)
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

			// Add outgoing message to queue
			sock.Channels.Outgoing <- msg
			msgBox.SetText("")
			go sock.Write()
		case tcell.KeyTab:
			msg := msgBox.GetText()
			msgBox.SetText(acHandler(sock, msg, acRE))
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
