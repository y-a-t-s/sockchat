package chat

import (
	"sync"
)

type ChatPool struct {
	msg  sync.Pool
	user sync.Pool
}

func newChatPool() *ChatPool {
	return &ChatPool{
		msg: sync.Pool{
			New: func() any {
				return new(Message)
			},
		},
		user: sync.Pool{
			New: func() any {
				return new(User)
			},
		},
	}
}

func (p *ChatPool) NewMsg() *Message {
	msg := p.msg.Get().(*Message)
	// Remove author pointer to prevent clearing underlying User record.
	msg.Author = nil
	*msg = Message{
		Author: p.NewUser(),
		pool:   p,
	}

	return msg
}

func (p *ChatPool) NewUser() *User {
	u := p.user.Get().(*User)
	*u = User{
		pool: p,
	}
	return u
}

func (p *ChatPool) Release(cd any) {
	switch cd.(type) {
	case *Message:
		p.msg.Put(cd)
	case *User:
		p.user.Put(cd)
	}
}
