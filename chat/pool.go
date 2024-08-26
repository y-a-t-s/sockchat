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
				return new(ChatMessage)
			},
		},
		user: &sync.Pool{
			New: func() any {
				return new(User)
			},
		},
	}
}

func (p *ChatPool) NewMsg() *ChatMessage {
	msg := p.msg.Get().(*ChatMessage)
	*msg = ChatMessage{
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
	case *ChatMessage:
		p.msg.Put(obj)
	case *User:
		p.user.Put(obj)
	}
}
