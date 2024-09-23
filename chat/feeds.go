package chat

import (
	"context"
)

func newFeedChan() chan *Message {
	return make(chan *Message, HIST_LEN)
}

type feed struct {
	Feed   chan Message
	closed chan struct{}
}

func newFeed() feed {
	return feed{
		Feed:   make(chan Message, HIST_LEN),
		closed: make(chan struct{}, 1),
	}
}

func (mc *feed) send(msg *Message) {
	select {
	case <-mc.closed:
		return
	default:
		// Not defined as a case to make sure mc.closed is checked first.
		mc.Feed <- *msg
	}

}

func (mf *feed) Close() {
	select {
	case <-mf.closed:
		return
	default:
		close(mf.closed)
		close(mf.Feed)
	}
}

type Feeder struct {
	in chan<- *Message

	Feed func() feed
}

func NewFeeder(ctx context.Context) *Feeder {
	in := newFeedChan()

	feeds := make([]feed, 0, 4)
	newFeeds := make(chan feed)

	fdr := &Feeder{
		in: in,
		Feed: func() feed {
			mf := newFeed()
			newFeeds <- mf
			return mf
		},
	}

	castMsg := func(msg *Message) {
		if msg == nil {
			return
		}

		for i := range feeds {
			select {
			case <-feeds[i].closed:
				// See if the closed feed can be replaced now.
				select {
				case feeds[i] = <-newFeeds:
					feeds[i].send(msg)
				default:
				}
			default:
				feeds[i].send(msg)
			}
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case mf := <-newFeeds:
				feeds = append(feeds, mf)
			case msg := <-in:
				castMsg(msg)
			}
		}
	}()

	return fdr
}

func (fdr *Feeder) Send(msg *Message) {
	fdr.in <- msg
}
