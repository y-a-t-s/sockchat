package socket

import (
	"fmt"
)

type User struct {
	ID        uint32 `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`

	// TODO: Hex colors with random bytes.
	// Color     [3]byte
}

type UserQuery struct {
	ID       string
	Username chan string
}

func (sock *ChatSocket) GetUsername(id string) string {
	q := UserQuery{
		ID:       id,
		Username: make(chan string, 2),
	}

	sock.Channels.UserQuery <- q
	if u := <-q.Username; u != "" {
		return u
	}

	// If the user data was not found for id, return the original match.
	return fmt.Sprint(id)
}

func (u *User) GetColor() string {
	// Original ANSI spec has 8 basic colors, which the user can set for their terminal theme.
	// 2 of the colors are black and white, which we'll leave out. This leaves 6 remaining colors.
	// We'll exclude the darker blue for better visibility against the standard dark background.
	ansi := []string{"red", "green", "yellow", "magenta", "cyan"}
	return ansi[u.ID%uint32(len(ansi))]
}

func (u *User) GetUserString() string {
	// This is kinda ugly, so I'll explain:
	// [%s::u] is a UI style tag for the color and enabling underlining.
	// [-] clears the set color to print the ID (the %d value).
	// [%s] sets the color back to the user's color to print the remaining "):".
	// [-::U] resets the color like before, while also disabling underlining to print the message.
	//
	// See https://github.com/rivo/tview/blob/master/doc.go for more info on style tags.
	return fmt.Sprintf("[%s::u]%s ([-]#%d[%s]):[-::U]", u.GetColor(), u.Username, u.ID, color)
}

func (sock *ChatSocket) UserHandler() {
	userMap := make(map[string]string)
	for {
		select {
		case user := <-sock.Channels.Users:
			userMap[fmt.Sprint(user.ID)] = user.Username
		case query := <-sock.Channels.UserQuery:
			query.Username <- userMap[query.ID]
		}
	}
}
