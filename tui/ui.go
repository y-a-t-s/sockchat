package tui

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"y-a-t-s/sockchat/config"
	"y-a-t-s/sockchat/services"
	"y-a-t-s/sockchat/socket"

	"github.com/gdamore/tcell/v2"
	"github.com/gen2brain/beeep"
	"github.com/rivo/tview"
)

type ui struct {
	services.Logger

	App      *tview.Application
	MainView *tview.Flex
	ChatView *tview.TextView
	InputBox *tview.InputField
}

func InitUI(ctx context.Context, c socket.Socket, cfg config.Config, l services.Logger) {
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

	u := ui{l, app, flex, chatView, msgBox}
	defer app.Stop()

	chatView.SetBorder(false)
	chatView.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyBacktab:
			flex.AddItem(msgBox, 1, 1, true)
			app.SetFocus(flex)
		case tcell.KeyCtrlC:
			app.Stop()
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

	go u.incomingHandler(ctx, c)
	app.Run()
}

func (u *ui) incomingHandler(ctx context.Context, c socket.Socket) {
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

	// Returns strings for UI and Logger respectively.
	formatMsg := func(msg *socket.ChatMessage) (string, string) {
		unEsc := escapeTags(html.UnescapeString(msg.MessageRaw))

		h, m, s := time.Unix(msg.MessageDate, 0).Clock()
		// Format timestamp with user's color.
		timestamp := fmt.Sprintf("%0.2d:%0.2d:%0.2d", h, m, s)

		// Format chat message, preceded by the sender's username and ID.
		uiStr := fmt.Sprintf("[%s::u]%s[-::U] %s [\"%d\"]%s[\"\"]\n",
			msg.Author.GetColor(), timestamp, msg.Author.GetUserString(),
			msg.MessageID, unEsc)

		// Format log message. Similar to ui formatting, but with
		// extra brackets for readability and no style tags.
		logStr := fmt.Sprintf("[%s] [%s (#%d)]: %s\n", timestamp,
			msg.Author.Username, msg.Author.ID, unEsc)

		return uiStr, logStr
	}

	var prev *socket.ChatMessage
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg := c.GetIncoming()
		if prev == nil || msg != *prev {
			msgStr, logStr := formatMsg(&msg)

			if u.Logger != nil {
				u.Log(logStr)
			}

			// Print chat message, preceded by the sender's username and ID.
			fmt.Fprint(u.ChatView, msgStr)
			u.ChatView.ScrollToEnd()
			mentionHandler(&msg)
		}

		prev = &msg
	}
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
