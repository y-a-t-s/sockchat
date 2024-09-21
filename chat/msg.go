package chat

import (
	"html"
	"time"
)

type Message struct {
	Author          *User  `json:"author"`
	Message         string `json:"message"`
	MessageRaw      string `json:"message_raw"`
	MessageID       uint32 `json:"message_id"`
	MessageDate     int64  `json:"message_date"`
	MessageEditDate int64  `json:"message_edit_date"`
	RoomID          uint16 `json:"room_id"`

	IsMention bool `json:"-"`
	debug     bool `json:"-"`

	pool *ChatPool
}

func (msg *Message) IsEdited() bool {
	return msg.MessageEditDate > 0
}

func (msg *Message) Release() {
	msg.Author = nil
	*msg = Message{
		pool: msg.pool,
	}

	msg.pool.Release(msg)
}

func (c *Chat) ClientMsg(body string, debug bool) {
	msg := c.pool.NewMsg()

	msg.Author.ID = 0
	msg.Author.Username = "sockchat"
	msg.MessageDate = time.Now().Unix()
	msg.Message = html.EscapeString(body)
	msg.MessageRaw = body
	msg.debug = debug

	c.sock.messages <- msg
}
