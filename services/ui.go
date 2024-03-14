package services

import (
	"context"
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

type chatUI struct {
	*tview.Application
	flex *tview.Flex

	chat     *chatView
	inputBox *tview.InputField

	logger Logger
	sock   socket.Socket
}

func InitUI(ctx context.Context, sock socket.Socket, cfg config.Config, logger Logger) {
	ui := chatUI{}

	ui.Application = tview.NewApplication()
	ui.flex = tview.NewFlex().SetDirection(tview.FlexRow)

	ui.chat = ui.newChatView()
	ui.flex.AddItem(ui.chat, 0, 1, false)
	if !cfg.ReadOnly {
		ui.inputBox = ui.newInputBox()
		ui.flex.AddItem(ui.inputBox, 1, 1, true)
	}

	ui.SetRoot(ui.flex, true).SetFocus(ui.flex)

	ui.logger = logger
	ui.sock = sock

	go ui.incomingHandler(ctx, sock)
	ui.Run()
}

func (ui *chatUI) newInputBox() *tview.InputField {
	ib := tview.NewInputField().
		SetAcceptanceFunc(tview.InputFieldMaxLength(1024)).
		SetFieldBackgroundColor(tcell.PaletteColor(0)).
		SetFieldWidth(0).
		SetLabel("> ")

	tabRE := regexp.MustCompile(`@(\d+)`)
	tabHandler := func(msg string) string {
		return tabRE.ReplaceAllStringFunc(msg, func(m string) string {
			id := m[1:]
			if u := ui.sock.QueryUser(id); u != id {
				return fmt.Sprintf("@%s,", u)
			}

			return m
		})
	}

	ib.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if ui.sock == nil {
				return
			}

			msg := strings.TrimSpace(ib.GetText())
			// Add outgoing message to queue.
			ui.sock.Send(msg)
			ib.SetText("")
		case tcell.KeyTab:
			msg := ib.GetText()
			ib.SetText(tabHandler(msg))
		case tcell.KeyBacktab:
			if ui.chat == nil || ui.flex == nil {
				return
			}

			ui.flex.RemoveItem(ib)
			ui.SetFocus(ui.chat)
		case tcell.KeyCtrlC:
			ui.Stop()
		}
	})

	ui.inputBox = ib
	return ib
}

func (ui *chatUI) incomingHandler(ctx context.Context, c socket.Socket) {
	tagRE := regexp.MustCompile(`\[(/?.+?)(="?(.*?)"?)?\]`)
	processTags := func(msg string) string {
		return tagRE.ReplaceAllStringFunc(msg, func(tag string) string {
			subs := tagRE.FindStringSubmatch(tag)
			tagName := subs[1]
			param := subs[3]

			switch lower := strings.ToLower(tagName); lower {
			case "b", "i", "s", "u":
				return fmt.Sprintf("[::%s]", lower)
			case "/b", "/i", "/s", "/u":
				return fmt.Sprintf("[::%s]", strings.ToUpper(lower[1:]))
			case "color":
				return fmt.Sprintf("[%s]", param)
			case "/color":
				return "[-]"
			default:
				// tview uses square brackets for formatting and region tags.
				// Any that appear in the raw message must be handled or escaped
				// by adding an extra opening bracket right before the closing one.
				return fmt.Sprintf("[%s[]", subs[1])
			}
		})
	}

	// IDs of any messages that mention the user. Used for message highlighting.
	var mentionIDs []string
	highlightMsg := func(msg *socket.ChatMessage) {
		beeep.Notify("New mention", html.UnescapeString(msg.MessageRaw), "")
		mentionIDs = append(mentionIDs, fmt.Sprint(msg.MessageID))
		ui.chat.Highlight(mentionIDs...)
	}

	var mentionRE *regexp.Regexp
	mentionHandler := func(msg *socket.ChatMessage) {
		if mentionRE == nil {
			clientName, err := c.ClientName()
			// GetClientName() returns err if client's user data has not been recorded yet.
			if err != nil {
				return
			}
			mentionRE = regexp.MustCompile(fmt.Sprintf("@%s,", clientName))
		}

		if mentionRE.MatchString(msg.MessageRaw) {
			highlightMsg(msg)
		}
	}

	var prev socket.ChatMessage
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg := c.IncomingMsg()
		// Don't output duplicates.
		if msg == prev {
			continue
		}

		timestamp := time.Unix(msg.MessageDate, 0)
		msgStr := html.UnescapeString(msg.MessageRaw)

		if ui.logger != nil {
			ui.logger.Log(fmt.Sprintf("[%s] [%s (#%d)]: %s\n",
				timestamp.Format("2006-01-02 15:04:05 MST"),
				msg.Author.Username, msg.Author.ID, msgStr))
		}

		// Print chat message, preceded by the sender's username and ID.
		fmt.Fprintf(ui.chat, "[%s::u]%s[-::U] %s [\"%d\"]%s[\"\"][-:-:-:-]\n",
			msg.Author.Color(), timestamp.Format("15:04:05"),
			msg.Author.UserString(), msg.MessageID, processTags(msgStr))
		mentionHandler(&msg)
		ui.chat.ScrollToEnd()

		prev = msg
	}
}
