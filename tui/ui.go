package tui

import (
	"fmt"
	"html"
	"os"
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

func InitUI(ssn *socket.Session) *UI {
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
			if !ssn.ChatDebug(msg) {
				// Add outgoing message to queue
				ssn.Outgoing <- []byte(msg)
			}
			msgBox.SetText("")
		case tcell.KeyTab:
			msg := msgBox.GetText()
			msgBox.SetText(tabHandler(ssn, msg))
		case tcell.KeyCtrlC:
			ssn.TryClose()
			app.Stop()
		}
	})
	msgBox.SetLabel("Message: ")

	flex.AddItem(chatView, 0, 1, false)
	flex.AddItem(msgBox, 1, 1, false)

	app.SetRoot(flex, true).SetFocus(msgBox)

	ui := &UI{app, flex, chatView, msgBox}
	go ui.incomingHandler(ssn)

	return ui
}

func tabHandler(ssn *socket.Session, msg string) string {
	return regexp.MustCompile(`@(\d+)`).ReplaceAllStringFunc(msg, func(m string) string {
		id := m[1:]
		if u := ssn.GetUsername(id); u != id {
			return fmt.Sprintf("@%s,", u)
		}

		return m
	})
}

func (ui *UI) incomingHandler(ssn *socket.Session) {
	tagRE := regexp.MustCompile(`\[(.+?)\]`)
	escapeTags := func(msg string) string {
		return tagRE.ReplaceAllString(msg, "[$1[]")
	}

	printTimestamp := func(msg *socket.ChatMessage) {
		h, m, s := time.Unix(msg.MessageDate, 0).Clock()
		// Print timestamp with user's color.
		fmt.Fprintf(ui.ChatView, "[%s::u]%0.2d:%0.2d:%0.2d[-::U] ", msg.Author.GetColor(), h, m, s)
	}

	var mentionRE *regexp.Regexp
	var mentionIDs []string
	mentionHandler := func(msg *socket.ChatMessage) {
		if mentionRE == nil {
			clientName := os.Getenv("SC_USER")
			if clientName == "" {
				return
			}

			mentionRE = regexp.MustCompile(fmt.Sprintf("@%s,", clientName))
		}

		if mentionRE.MatchString(msg.MessageRaw) {
			mentionIDs = append(mentionIDs, fmt.Sprint(msg.MessageID))
			ui.ChatView.Highlight(mentionIDs...)
		}
	}

	var prev *socket.ChatMessage
	for {
		msg := <-ssn.Messages
		if prev == nil || msg != *prev {
			unEsc := escapeTags(html.UnescapeString(msg.MessageRaw))

			printTimestamp(&msg)
			// Print chat message, preceded by the sender's username and ID.
			fmt.Fprintf(ui.ChatView, "%s [\"%d\"]%s[\"\"]\n", msg.Author.GetUserString(), msg.MessageID, unEsc)

			ui.ChatView.ScrollToEnd()
			mentionHandler(&msg)
		}

		prev = &msg
	}
}
