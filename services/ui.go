package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"y-a-t-s/sockchat/chat"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TUI struct {
	*tview.Application

	flex     *tview.Flex
	Console  *tview.TextView
	inputBox *tview.InputField

	Chat *chat.Chat
}

func StartTUI(ctx context.Context, c *chat.Chat) {
	ui := TUI{}

	ui.Application = tview.NewApplication()
	ui.Chat = c
	ui.flex = tview.NewFlex().SetDirection(tview.FlexRow)

	ui.Console = tview.NewTextView().
		SetDynamicColors(true).
		SetMaxLines(chat.HIST_LEN).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			ui.Draw()
		})
	// Returns *tview.Box, so keep separate from assignment.
	ui.Console.SetBorder(false)
	ui.Console.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyBacktab:
			if ui.flex == nil || ui.inputBox == nil {
				return
			}

			ui.flex.AddItem(ui.inputBox, 1, 1, true)
			ui.SetFocus(ui.flex)
		case tcell.KeyCtrlC:
			ui.Stop()
		}
	})

	ui.flex.AddItem(ui.Console, 0, 1, false)
	if !ui.Chat.Cfg.ReadOnly {
		ui.inputBox = ui.newInputBox()
		ui.flex.AddItem(ui.inputBox, 1, 1, true)
	}

	ui.SetRoot(ui.flex, true).SetFocus(ui.flex)

	go ui.incomingHandler(ctx)
	ui.Run()
}

func (ui *TUI) newInputBox() *tview.InputField {
	ib := tview.NewInputField().
		// Idk what the site caps it at.
		SetAcceptanceFunc(tview.InputFieldMaxLength(2048)).
		// Use terminal background color for input box.
		SetFieldBackgroundColor(tcell.PaletteColor(0)).
		SetFieldWidth(0).
		SetLabel("> ")

	tabHandler := func(msg string) string {
		return regexp.MustCompile(`@(\d+)`).ReplaceAllStringFunc(msg, func(m string) string {
			id, err := strconv.Atoi(m[1:])
			if err != nil {
				// TODO: Better handling.
				return ""
			}

			if u := ui.Chat.Users.Query(uint32(id)); u != nil {
				return fmt.Sprintf("@%s,", u.Username)
			}

			return m
		})
	}

	ib.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			msg := strings.TrimSpace(ib.GetText())
			// Add outgoing message to queue.
			ui.Chat.Out <- msg
			ib.SetText("")
		case tcell.KeyTab:
			msg := ib.GetText()
			ib.SetText(tabHandler(msg))
		case tcell.KeyBacktab:
			if ui.Console == nil || ui.flex == nil {
				return
			}

			ui.flex.RemoveItem(ib)
			ui.SetFocus(ui.Console)
		case tcell.KeyCtrlC:
			ui.Stop()
		}
	})

	ui.inputBox = ib
	return ib
}

func (ui *TUI) incomingHandler(ctx context.Context) {
	// Process BBCode tags. Currently limited to formatting.
	processTags := func(msg string) string {
		tagRE := regexp.MustCompile(`\[(/?.+?)(="?(.*?)"?)?\]`)
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

	// IDs of any messages that mention the user. Used for highlighting.
	var mentionIDs []string
	highlightMsg := func(msg *chat.Message) {
		mentionIDs = append(mentionIDs, fmt.Sprint(msg.MessageID))
		ui.Console.Highlight(mentionIDs...)
	}

	// Generate output string for msg.
	msgStr := func(msg *chat.Message) string {
		fl := ""
		if msg.MessageEditDate != 0 {
			fl = "[::d]*[::D]"
		}

		// Print chat message, preceded by the sender's username and ID.
		return fmt.Sprintf("[%s::u]%s[-::U] %s [\"%d\"]%s[\"\"][-:-:-:-]\n",
			msg.Author.Color(), time.Unix(msg.MessageDate, 0).Format("15:04:05"),
			msg.Author.String(fl), msg.MessageID, processTags(msg.MessageRaw))
	}

	var (
		// Buffer for chat history rewrites.
		bb bytes.Buffer
		// Chat msg feed from socket.
		feed = ui.Chat.NewFeed()
	)

	var mre *regexp.Regexp
	for {
		select {
		case <-ctx.Done():
			ui.Stop()
			return
		case hc := <-ui.Chat.History:
			if hc == nil {
				continue
			}

			n := 0
			for msg := range hc {
				if msg != nil {
					bb.WriteString(msgStr(msg))
					n++

					if msg.IsMention {
						highlightMsg(msg)
					}
				}
			}

			// Clear before redrawing if we don't have enough
			// to completely overwrite the history.
			if n < chat.HIST_LEN {
				ui.Console.Clear()
			}

			bb.WriteTo(ui.Console)
			ui.Console.ScrollToEnd()
			ui.Console.Highlight(mentionIDs...)
			bb.Reset()
		case msg := <-feed:
			if msg == nil {
				continue
			}

			io.WriteString(ui.Console, msgStr(msg))
			if un := ui.Chat.Client.Username; mre == nil && un != "" {
				mre = regexp.MustCompile(fmt.Sprintf("@%s,", un))
			}
			if mre != nil && mre.MatchString(msg.MessageRaw) {
				highlightMsg(msg)
			}
			ui.Console.ScrollToEnd()
		}
	}
}
