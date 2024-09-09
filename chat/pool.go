package chat

import (
	"sync"
)

type ChatPool struct {
	msg  *sync.Pool
	user *sync.Pool
}

func newChatPool() ChatPool {
	return ChatPool{
		msg: &sync.Pool{
			New: func() any {
				return new(Message)
			},
		},
		user: &sync.Pool{
			New: func() any {
				return new(User)
			},
		},
	}
}

func (p *ChatPool) NewMsg() *Message {
	msg := p.msg.Get().(*Message)
	*msg = Message{
		Author: p.NewUser(),
	}

	return msg
}

func (p *ChatPool) NewUser() *User {
	u := p.user.Get().(*User)
	if u.color != "" {
		*u = User{}
	}
	return u
}

func (p *ChatPool) Release(obj interface{}) {
	switch obj.(type) {
	case *Message:
		p.msg.Put(obj)
	case *User:
		p.user.Put(obj)
	}
}
