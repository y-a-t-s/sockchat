package chat

import (
	"fmt"
	"math/rand"
	"strings"
)

type User struct {
	ID        uint32 `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`

	color string
}

func (u *User) Color() string {
	if u.color == "" {
		u.SetColor()
	}

	return u.color
}

func (u *User) SetColor() {
	rng := rand.New(rand.NewSource(int64(u.ID)))

	// Original ANSI spec has 8 basic colors, which the user can set for their terminal theme.
	// 2 of the colors are black and white, which we'll leave out. This leaves 6 remaining colors.
	// We'll exclude the darker blue for better visibility against the standard dark background.
	ansi := []string{"red", "green", "yellow", "magenta", "cyan"}
	n := len(ansi)

	u.color = ansi[uint32(rng.Int()%n)]
}

func (u *User) String(fl string) string {
	color := u.Color()
	// This is kinda ugly, so I'll explain:
	// [%s::u] is a UI style tag for the color and enabling underlining.
	// [-] clears the set color to print the ID (the %d value).
	// [%s] sets the color back to the user's color to print the remaining "):".
	// [-::U] resets the color like before, while also disabling underlining to print the message.
	//
	// See https://github.com/rivo/tview/blob/master/doc.go for more info on style tags.
	return fmt.Sprintf("[%s::u]%s ([-]#%d[%s])%s:[-::U]", color, strings.ReplaceAll(u.Username, "]", "[]"), u.ID, color, fl)
}

type userQuery struct {
	id  uint32
	res chan<- *User
}

type userTable struct {
	in      chan<- *User
	queries chan userQuery
}

func newUserTable() *userTable {
	ut := &userTable{
		queries: make(chan userQuery, 4),
	}
	ut.in = ut.run()
	return ut
}

func (ut *userTable) run() chan<- *User {
	in := make(chan *User, 64)
	table := make(map[uint32]*User, 256)

	go func() {
		for {
			select {
			case u, ok := <-in:
				switch {
				case !ok:
					return
				case u == nil:
					continue
				}

				if table[u.ID] == nil {
					table[u.ID] = u
				}
			case uq := <-ut.queries:
				uq.res <- table[uq.id]
				close(uq.res)
			}
		}
	}()

	return in
}

func (ut *userTable) Add(u *User) bool {
	if ut.Query(u.ID) != nil {
		return false
	}

	if u != nil {
		ut.in <- u
	}

	return true
}

func (ut *userTable) Query(id uint32) *User {
	res := make(chan *User, 2)
	ut.queries <- userQuery{id, res}
	return <-res
}
