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

func ClientMsg(body string) *ChatMessage {
	return &ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageDate: time.Now().Unix(),
		Message:     html.EscapeString(body),
		MessageRaw:  body,
	}
}

func (msg *ChatMessage) IsEdited() bool {
	return msg.MessageEditDate > 0
}

func (msg *ChatMessage) Reset() {
	msg.Author = User{}
	msg.Message = ""
	msg.MessageRaw = ""
	msg.MessageID = 0
	msg.MessageDate = 0
	msg.MessageEditDate = 0
	msg.RoomID = 0
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
