package chat

type User struct {
	ID        uint32 `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type ChatMessage struct {
	Author          User   `json:"author"`
	Message         string `json:"message"`
	MessageRaw      string `json:"message_raw"`
	MessageID       uint32 `json:"message_id"`
	MessageDate     uint64 `json:"message_date"`
	MessageEditDate uint64 `json:"message_edit_date"`
	RoomID          uint16 `json:"room_id"`
}

type SocketMessage struct {
	Messages []ChatMessage   `json:"messages"`
	Users    map[string]User `json:"users"`
}

func (sock *ChatSocket) ClientMsg(msg string) {
	sock.Received <- ChatMessage{
		Author: User{
			ID:       0,
			Username: "sockchat",
		},
		MessageRaw: msg,
	}
}
