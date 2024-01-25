package chat

import (
	"fmt"
	"html"
	"regexp"

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
		sock.Conn.Close()
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
	go c.Socket.Write()

	return c
}

func (c *Chat) userHandler() {
	userMap := make(map[string]string)
	for {
		select {
		case user := <-c.Socket.Channels.Users:
			userMap[fmt.Sprint(user.ID)] = user.Username
		case query := <-c.Socket.Channels.UserQuery:
			query.Username <- userMap[query.ID]
		}
	}
}

func escapeTags(msg string) string {
	return regexp.MustCompile(`\[(.+?)\]`).ReplaceAllString(msg, "[$1[]")
}

func (c *Chat) msgHandler(prev *socket.ChatMessage) *Chat {
	msg := <-c.Socket.Channels.Messages
	if prev == nil || msg != *prev {
		un := escapeTags(html.UnescapeString(msg.MessageRaw))
		fmt.Fprintf(c.UI.ChatView, "%s %s\n", msg.Author.GetUserString(), un)
		c.UI.ChatView.ScrollToEnd()
	}

	// Socket reads sometimes return messages we fetched previously.
	// A simple recursive comparison with the previous printed message handles it well enough.
	return c.msgHandler(&msg)
}
