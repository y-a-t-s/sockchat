package chat

import (
	"fmt"
	"html"

	"y-a-t-s/sockchat/socket"
	"y-a-t-s/sockchat/tui"
)

type Chat struct {
	Socket *socket.ChatSocket
	UI     *tui.UI
}

func InitChat() *Chat {
	sock := socket.NewSocket()
	if sock == nil {
		return nil
	}

	ui := tui.InitUI(sock)
	if ui == nil {
		sock.Conn.CloseNow()
		return nil
	}

	return &Chat{
		Socket: sock,
		UI:     ui,
	}
}

func (c *Chat) FetchMessages() *Chat {
	go c.msgHandler(nil)
	go c.userHandler()
	go c.Socket.Fetch()

	return c
}

func (c *Chat) userHandler() *Chat {
	for {
		user := <-c.Socket.Channels.Users
		id := fmt.Sprint(user.ID)

		// Just to be safe.
		c.Socket.Users.Mutex.Lock()
		c.Socket.Users.UserMap[id] = user.Username
		c.Socket.Users.Mutex.Unlock()
	}
}

func (c *Chat) msgHandler(prev *socket.ChatMessage) *Chat {
	msg := <-c.Socket.Channels.Messages
	if prev == nil || msg != *prev {
		fmt.Fprintf(c.UI.ChatView, "%s %s\n", msg.Author.GetUserString(), html.UnescapeString(msg.MessageRaw))
		c.UI.ChatView.ScrollToEnd()
	}

	// Socket reads sometimes return messages we fetched previously.
	// A simple recursive comparison with the previous printed message handles it well enough.
	return c.msgHandler(&msg)
}
