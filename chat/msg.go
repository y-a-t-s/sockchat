package chat

import (
	"html"
	"sync"
	"time"
)

type ChatMessage struct {
	Author          User   `json:"author"`
	Message         string `json:"message"`
	MessageRaw      string `json:"message_raw"`
	MessageID       uint32 `json:"message_id"`
	MessageDate     int64  `json:"message_date"`
	MessageEditDate int64  `json:"message_edit_date"`
	RoomID          uint16 `json:"room_id"`
}

func (msg *ChatMessage) IsEdited() bool {
	return msg.MessageEditDate > 0
}

func (msg *ChatMessage) Reset() {
	*msg = ChatMessage{}
}

type msgPool struct {
	pool *sync.Pool
}

func newMsgPool() msgPool {
	return msgPool{
		&sync.Pool{
			New: func() any {
				return new(ChatMessage)
			},
		},
	}
}

func (mp *msgPool) NewMsg() *ChatMessage {
	return mp.pool.Get().(*ChatMessage)
}

func (mp *msgPool) Release(msg *ChatMessage) {
	mp.pool.Put(msg)
}

func (s *sock) ClientMsg(body string) {
	msg := s.pool.NewMsg()
	*msg = ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageDate: time.Now().Unix(),
		Message:     html.EscapeString(body),
		MessageRaw:  body,
	}

	s.messages <- msg
}
