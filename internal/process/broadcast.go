package process

import (
	"sync"
)

type Broadcaster struct {
	mu          sync.RWMutex
	nextSubID   int
	subscribers map[int]chan []byte
	closed      bool
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[int]chan []byte),
	}
}

func (b *Broadcaster) Subscribe() (int, chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan []byte)
		close(ch)
		return -1, ch
	}

	id := b.nextSubID
	b.nextSubID++

	ch := make(chan []byte, 256)
	b.subscribers[id] = ch
	return id, ch
}

func (b *Broadcaster) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.subscribers[id]
	if ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

func (b *Broadcaster) Send(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed || len(data) == 0 {
		return
	}

	// Copy data to ensure the receiver goroutine receives a stable slice
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	for _, ch := range b.subscribers {
		select {
		case ch <- dataCopy:
		default:
			// slow consumer: drop this block of log output
		}
	}
}

func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = make(map[int]chan []byte)
}
