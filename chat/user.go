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
	pool  *ChatPool
}

func (u *User) Color() string {
	if u.color == "" {
		// Just send back something instead of risking a data race trying to set it.
		return "green"
	}

	return u.color
}

func (u *User) Release() {
	*u = User{
		pool: u.pool,
	}
	if u.pool != nil {
		u.pool.Release(u)
	}
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
	// See https://github.com/rivo/tview/blob/master/doc.go for more info on style tags.
	return fmt.Sprintf("[%s::u]%s ([-]#%d[%s])%s:[-::U]", color, strings.ReplaceAll(u.Username, "]", "[]"), u.ID, color, fl)
}

type clientUser struct {
	ID       uint32
	Username string
	found    chan struct{}
}

func newClientUser(id uint32) clientUser {
	return clientUser{
		ID:    id,
		found: make(chan struct{}),
	}
}

type userTable struct {
	sync.Map
	Client clientUser
}

// TODO: Make this value passing less stupid.
func NewUserTable(clientID uint32) *userTable {
	return &userTable{
		Client: newClientUser(clientID),
	}
}

func (ut *userTable) ClientName() string {
	select {
	case <-ut.Client.found:
		return ut.Client.Username
	default:
		return ""
	}
}

func (ut *userTable) AddUser(u *User) *User {
	select {
	case <-ut.Client.found:
	default:
		if ut.Client.ID == u.ID {
			ut.Client.Username = u.Username
			close(ut.Client.found)
		}
	}

	// Ensure we don't store User sourced from pool.
	nu := *u
	// nu := &(*u) // TODO: See if this actually allocates a new obj.
	nu.SetColor()

	lui, loaded := ut.LoadOrStore(nu.ID, &nu)
	lu := lui.(*User)

	switch {
	case lu == nil:
		// If this somehow happens, there's a BIG fucking problem.
		panic("userTable loaded nil *User.")
	case loaded:
		if nu != *lu {
			ut.Store(u.ID, &nu)
			return &nu
		}
	}

	return lu
}

func (ut *userTable) Query(id uint32) *User {
	u, ok := ut.Load(id)
	if !ok {
		return nil
	}

	return u.(*User)
}
