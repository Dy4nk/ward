package process

import (
	"bytes"
	"sync"
)

type lineSplitter struct {
	accum  []byte
	onLine func([]byte)
}

func newLineSplitter(onLine func([]byte)) *lineSplitter {
	return &lineSplitter{
		onLine: onLine,
	}
}

func (ls *lineSplitter) Write(data []byte) {
	ls.accum = append(ls.accum, data...)
	for {
		idx := bytes.IndexByte(ls.accum, '\n')
		if idx < 0 {
			break
		}
		line := ls.accum[:idx]
		
		// Copy line slice to guarantee stability before invoking callback
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		
		ls.onLine(lineCopy)
		ls.accum = ls.accum[idx+1:]
	}
}

type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	head     int
	lines    [][]byte
	size     int
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 500
	}
	return &RingBuffer{
		capacity: capacity,
		lines:    make([][]byte, capacity),
	}
}

func (rb *RingBuffer) WriteLine(line []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.lines[rb.head] = line
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

func (rb *RingBuffer) Read(n int) [][]byte {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return nil
	}

	if n <= 0 || n > rb.size {
		n = rb.size
	}

	result := make([][]byte, n)
	startIdx := (rb.head - rb.size + rb.capacity) % rb.capacity
	skip := rb.size - n
	startIdx = (startIdx + skip) % rb.capacity

	for i := 0; i < n; i++ {
		idx := (startIdx + i) % rb.capacity
		result[i] = rb.lines[idx]
	}

	return result
}

func (rb *RingBuffer) ReadJoined(n int) []byte {
	lines := rb.Read(n)
	if len(lines) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}
