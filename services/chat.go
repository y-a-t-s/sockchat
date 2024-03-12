package services

import (
	"io"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type chatView struct {
	*tview.TextView
}

func (ui *chatUI) newChatView() *chatView {
	chat := tview.NewTextView().
		SetDynamicColors(true).
		SetMaxLines(2048).
		SetRegions(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			ui.Draw()
		})
	// Returns *tview.Box, so keep separate from assignment.
	chat.SetBorder(false)
	chat.SetDoneFunc(func(key tcell.Key) {
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

	ui.chat = &chatView{chat}
	return ui.chat
}

func (cv *chatView) Read(p []byte) (n int, err error) {
	chatTxt := cv.GetText(false)
	if chatTxt == "" {
		return 0, io.EOF
	}

	n = copy(p, []byte(chatTxt))
	err = nil
	return
}
