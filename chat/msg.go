package chat

import (
	"html"
	"time"
)

type ChatMessage struct {
	Author          *User  `json:"author"`
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

func (c *Chat) ClientMsg(body string) {
	u := c.pool.NewUser()
	*u = User{
		ID:       0,
		Username: "sockchat",
	}

	msg := c.pool.NewMsg()
	*msg = ChatMessage{
		Author:      u,
		MessageDate: time.Now().Unix(),
		Message:     html.EscapeString(body),
		MessageRaw:  body,
	}

	c.sock.messages <- msg
}
