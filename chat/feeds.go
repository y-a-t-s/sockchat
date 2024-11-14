package chat

import (
	"context"
	"sync"
)

func newFeedChan() chan *Message {
	return make(chan *Message, HIST_LEN)
}

type feed struct {
	// Signals closed feed. Similar to ctx.Done()
	closed chan struct{}

	Feed chan Message
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

type feeder struct {
	in chan *Message

	Feed func() feed
}

func newFeeder(ctx context.Context) feeder {
	feeds := make([]feed, 0, 4)
	newFeeds := make(chan feed)
	closed := make(chan feed, 4)

	fdr := feeder{
		in: newFeedChan(),
		Feed: func() feed {
			mf := newFeed()

			// See if a previously closed feed can be replaced in the slice.
			select {
			case cmf := <-closed:
				// chans are passed by ref, so this is probably ok.
				cmf.Feed = mf.Feed
				cmf.closed = mf.closed
			default:
				newFeeds <- mf
			}

			return mf
		},
	}

	broadcast := func(msg *Message) {
		if msg == nil {
			return
		}

		for _, mf := range feeds {
			select {
			case <-mf.closed:
				select {
				case closed <- mf:
				default:
				}
			default:
				mf.send(msg)
			}
		}
	}

	closeAll := sync.OnceFunc(func() {
		for _, mf := range feeds {
			mf.Close()
		}
	})

	go func() {
		defer closeAll()
		for {
			select {
			case <-ctx.Done():
				closeAll()
				return
			case mf := <-newFeeds:
				feeds = append(feeds, mf)
			case msg, ok := <-fdr.in:
				if !ok {
					return
				}
				broadcast(msg)
			}
		}
	}()

	return fdr
}

func (fdr *feeder) Send(msg *Message) {
	fdr.in <- msg
}
