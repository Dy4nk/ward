package process

import (
	"testing"
	"time"
)

func TestBroadcaster_Basic(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	id1, ch1 := b.Subscribe()
	_, ch2 := b.Subscribe()

	data := []byte("hello log stream")
	b.Send(data)

	// Verify both subscribers receive it
	select {
	case d := <-ch1:
		if string(d) != string(data) {
			t.Errorf("ch1 expected %q, got %q", string(data), string(d))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 timed out waiting for broadcast")
	}

	select {
	case d := <-ch2:
		if string(d) != string(data) {
			t.Errorf("ch2 expected %q, got %q", string(data), string(d))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 timed out waiting for broadcast")
	}

	b.Unsubscribe(id1)

	data2 := []byte("second msg")
	b.Send(data2)

	// ch1 should be closed, so receiving from it returns immediately
	_, ok := <-ch1
	if ok {
		t.Error("ch1 expected to be closed after unsubscribe")
	}

	// ch2 should receive the second message
	select {
	case d := <-ch2:
		if string(d) != string(data2) {
			t.Errorf("ch2 expected %q, got %q", string(data2), string(d))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 timed out waiting for second broadcast")
	}
}

func TestBroadcaster_SlowConsumer(t *testing.T) {
	b := NewBroadcaster()
	defer b.Close()

	// Subscribe, but don't read from channel
	_, ch := b.Subscribe()

	for i := 0; i < 256; i++ {
		b.Send([]byte{byte(i)})
	}

	// This 257th Send should not block and should drop the packet for this subscriber
	done := make(chan struct{})
	go func() {
		b.Send([]byte("dropped message"))
		close(done)
	}()

	select {
	case <-done:
		// Passed: Send completed without blocking
	case <-time.After(500 * time.Millisecond):
		t.Error("Send blocked on slow consumer")
	}

	for i := 0; i < 256; i++ {
		d := <-ch
		if len(d) != 1 || d[0] != byte(i) {
			t.Fatalf("unexpected message at index %d: %v", i, d)
		}
	}

	// The next read should block or be empty (if there are no other messages)
	select {
	case d := <-ch:
		t.Errorf("received unexpected message that should have been dropped: %q", string(d))
	default:
		// Correct: the dropped message is indeed not in the channel
	}
}

func TestBroadcaster_Close(t *testing.T) {
	b := NewBroadcaster()
	_, ch := b.Subscribe()

	b.Close()

	_, ok := <-ch
	if ok {
		t.Error("channel expected to be closed after Broadcaster.Close()")
	}

	// Subscribing to a closed broadcaster should return closed channel
	_, ch2 := b.Subscribe()
	_, ok = <-ch2
	if ok {
		t.Error("channel from closed broadcaster should be closed immediately")
	}
}
