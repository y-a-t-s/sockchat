package chat

import (
	"context"
	"sync"
)

type feed struct {
	Feed   chan Message
	closed chan struct{}
}

func newFeed() *feed {
	return &feed{
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
	feeds []*feed
	in    chan<- *Message

	mx   sync.RWMutex
	once sync.Once
}

func NewFeeder(ctx context.Context) *Feeder {
	in := make(chan *Message, 512)
	fdr := &Feeder{
		feeds: make([]*feed, 0, 4),
		in:    in,
	}

	dropFeed := func(i int) {
		fdr.mx.Lock()
		defer fdr.mx.Unlock()

		fl := len(fdr.feeds) - 1
		if i > fl {
			return
		}

		fdr.feeds[i].Close()
		if i < fl {
			fdr.feeds[i] = fdr.feeds[fl]
		}
		fdr.feeds = fdr.feeds[:fl-1]
	}

	castMsg := func(msg *Message) {
		if msg == nil {
			return
		}

		fdr.mx.RLock()
		defer fdr.mx.RUnlock()

		for i, mf := range fdr.feeds {
			select {
			case <-mf.closed:
				go dropFeed(i)
			default:
				mf.send(msg)
			}
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-in:
				castMsg(msg)
			}
		}
	}()

	return fdr
}

func (fdr *Feeder) NewFeed() *feed {
	mf := newFeed()

	go func() {
		fdr.mx.Lock()
		defer fdr.mx.Unlock()

		fdr.feeds = append(fdr.feeds, mf)
	}()

	return mf
}

func (fdr *Feeder) Send(msg *Message) {
	fdr.in <- msg
}
