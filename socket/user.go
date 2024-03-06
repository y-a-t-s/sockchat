package socket

import (
	"fmt"
	"os"
	"strconv"
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

func (u User) GetColor() string {
	// Original ANSI spec has 8 basic colors, which the user can set for their terminal theme.
	// 2 of the colors are black and white, which we'll leave out. This leaves 6 remaining colors.
	// We'll exclude the darker blue for better visibility against the standard dark background.
	ansi := []string{"red", "green", "yellow", "magenta", "cyan"}
	return ansi[u.ID%uint32(len(ansi))]
}

func (u User) GetUserString() string {
	color := u.GetColor()
	// This is kinda ugly, so I'll explain:
	// [%s::u] is a UI style tag for the color and enabling underlining.
	// [-] clears the set color to print the ID (the %d value).
	// [%s] sets the color back to the user's color to print the remaining "):".
	// [-::U] resets the color like before, while also disabling underlining to print the message.
	//
	// See https://github.com/rivo/tview/blob/master/doc.go for more info on style tags.
	return fmt.Sprintf("[%s::u]%s ([-]#%d[%s]):[-::U]", color, u.Username, u.ID, color)
}

func (sock Socket) QueryUser(id string) string {
	q := UserQuery{
		ID:       id,
		Username: make(chan string, 2),
	}

	sock.UserQuery <- q
	if u := <-q.Username; u != "" {
		return u
	}

	// If the user data was not found for id, return the original match.
	return id
}

func (sock *Socket) userHandler() {
	clientId := func() *uint32 {
		if idStr := os.Getenv("SC_USER_ID"); idStr != "" {
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return nil
			}

			ui := uint32(id)
			return &ui
		}

		return nil
	}()

	userMap := make(map[string]string)
	for {
		select {
		case user := <-sock.users:
			if clientId != nil && user.ID == *clientId {
				os.Setenv("SC_USER", user.Username)
				// Probably helps efficiency down the line.
				clientId = nil
			}
			userMap[fmt.Sprint(user.ID)] = user.Username
		case query := <-sock.UserQuery:
			query.Username <- userMap[query.ID]
		}
	}
}
