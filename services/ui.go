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

const HISTORY_LEN uint8 = 4

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
		SetScrollable(true)

	// When scrolled to the bottom, the textview will auto-scroll as msgs come in.
	ui.Console.ScrollToEnd()
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
		case tcell.KeyPgDn:
			err := ui.Chat.Reconnect(ctx)
			if err != nil {
				panic(err)
			}
		}
	})

	ui.flex.AddItem(ui.Console, 0, 1, false)
	// Don't enable msg input box in RO mode.
	if !c.Cfg.ReadOnly {
		ui.inputBox = ui.newInputBox(ctx)
		ui.flex.AddItem(ui.inputBox, 1, 1, true)
	}

	ui.SetRoot(ui.flex, true).SetFocus(ui.flex)

	go ui.incomingHandler(ctx)
	ui.Run()
}

func (ui *TUI) newInputBox(ctx context.Context) *tview.InputField {
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
				ui.Chat.Errs <- err
				return m
			}

			if u := ui.Chat.Users.Query(uint32(id)); u != nil {
				return fmt.Sprintf("@%s,", u.Username)
			}

			return m
		})
	}

	var History struct {
		hist []string
		Add  func(msg string) bool
	}
	History.hist = make([]string, 0, HISTORY_LEN)
	// TODO: Probably turn this into an error return type with some custom error.
	History.Add = func(msg string) bool {
		if msg == "" {
			return false
		}

		History.hist = append(History.hist, msg)
		if len(History.hist) > int(HISTORY_LEN) {
			// Pop oldest msg from slice.
			History.hist = History.hist[1:]
		}

		return true
	}

	var histIdx uint8
	ib.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			msg := strings.TrimSpace(ib.GetText())
			if msg == "" {
				return
			}

			// Add outgoing message to queue.
			ui.Chat.Out <- msg
			ib.SetText("")

			History.Add(msg)
		case tcell.KeyBacktab:
			if ui.Console == nil || ui.flex == nil {
				return
			}

			ui.flex.RemoveItem(ib)
			ui.SetFocus(ui.Console)
		case tcell.KeyCtrlC:
			ui.Stop()
		case tcell.KeyPgDn:
			err := ui.Chat.Reconnect(ctx)
			if err != nil {
				panic(err)
			}
		case tcell.KeyTab:
			msg := ib.GetText()
			ib.SetText(tabHandler(msg))
		case tcell.KeyUp:
			hl := len(History.hist)
			if int(histIdx) > hl {
				return
			}

			ib.SetText(History.hist[hl-int(histIdx)])
			histIdx++
		case tcell.KeyDown:
			if histIdx == 0 {
				return
			}

			ib.SetText(History.hist[histIdx])
			ib.SetText(History.hist[len(History.hist)-int(histIdx)])
			histIdx--
		}
	})

	ui.inputBox = ib
	return ib
}

func (ui *TUI) incomingHandler(ctx context.Context) {
	defer ui.Stop()

	var (
		bb         bytes.Buffer // Buffer for quick chat history rewrites.
		mentionIDs []string     // IDs of msgs that mention the user. Used for highlighting.

		prevID uint32 = 0
	)

	ui.Console.SetChangedFunc(func() {
		ui.Draw()
	})

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

	highlight := func() {
		ui.QueueUpdateDraw(func() {
			ui.Console.Highlight(mentionIDs...)
		})
	}

	// Chat msg feed from socket.
	feed := ui.Chat.Feeder.Feed()
	defer feed.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case hc := <-ui.Chat.History:
			if hc == nil {
				continue
			}

			n := 0
			mentionIDs = make([]string, 0, len(mentionIDs))
			for msg := range hc {
				id := msg.MessageID

				bb.WriteString(msgStr(&msg))
				n++

				if msg.IsMention {
					mentionIDs = append(mentionIDs, fmt.Sprint(id))
				}
			}

			ui.Console.Clear()
			ui.Console.ScrollToEnd()
			bb.WriteTo(ui.Console)
			bb.Reset()
			highlight()
		case msg := <-feed.Feed:
			if msg.IsEdited() && msg.MessageID <= prevID {
				continue
			}

			io.WriteString(ui.Console, msgStr(&msg))
			if msg.IsMention {
				mentionIDs = append(mentionIDs, fmt.Sprint(msg.MessageID))
				highlight()
			}

			prevID = msg.MessageID
		}
	}
}
