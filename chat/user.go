package chat

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
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

type userTable struct {
	table map[uint32]*User
	mutex sync.Mutex
	pool  sync.Pool
}

func newUserTable() *userTable {
	return &userTable{
		table: make(map[uint32]*User, 256),
		pool: sync.Pool{
			New: func() any {
				return new(User)
			},
		},
	}
}

func (ut *userTable) NewUser() *User {
	u := ut.pool.Get().(*User)

	if u.color != "" {
		*u = User{}
	}

	return u
}

func (ut *userTable) ReleaseUser(u *User) {
	ut.pool.Put(u)
}

func (ut *userTable) Add(u *User) bool {
	ut.mutex.Lock()
	defer ut.mutex.Unlock()

	if ut.table[u.ID] == nil {
		ut.table[u.ID] = u
		return true
	}

	return false
}

func (ut *userTable) Query(id uint32) *User {
	ut.mutex.Lock()
	defer ut.mutex.Unlock()

	return ut.table[id]
}
