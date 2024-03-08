package tui

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/socket"

	"github.com/gdamore/tcell/v2"
	"github.com/gen2brain/beeep"
	"github.com/rivo/tview"
)

type UI struct {
	App      *tview.Application
	MainView *tview.Flex
	ChatView *tview.TextView
	InputBox *tview.InputField
}

func InitUI(c socket.Socket, cfg config.Config) UI {
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

	chatView.SetBorder(false)
	chatView.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyBacktab:
			flex.AddItem(msgBox, 1, 1, true)
			app.SetFocus(flex)
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
			app.Stop()
		}
	})

	flex.AddItem(chatView, 0, 1, false)
	if !cfg.ReadOnly {
		flex.AddItem(msgBox, 1, 1, true)
	}

	app.SetRoot(flex, true).SetFocus(flex)

	ui := UI{app, flex, chatView, msgBox}
	go ui.incomingHandler(c)

	return ui
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

func (ui *UI) incomingHandler(c socket.Socket) {
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
			ui.ChatView.Highlight(mentionIDs...)
		}
	}

	var prev *socket.ChatMessage
	for {
		msg := c.GetIncoming()
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
