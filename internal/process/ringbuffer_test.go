package process

import (
	"testing"
)

func TestRingBuffer_Basic(t *testing.T) {
	rb := NewRingBuffer(3)

	rb.WriteLine([]byte("line 1"))
	rb.WriteLine([]byte("line 2"))

	lines := rb.Read(5)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if string(lines[0]) != "line 1" || string(lines[1]) != "line 2" {
		t.Errorf("incorrect lines read: %s, %s", string(lines[0]), string(lines[1]))
	}

	joined := rb.ReadJoined(5)
	expectedJoined := "line 1\nline 2\n"
	if string(joined) != expectedJoined {
		t.Errorf("expected joined %q, got %q", expectedJoined, string(joined))
	}
}

func TestRingBuffer_Wrapping(t *testing.T) {
	rb := NewRingBuffer(3)

	rb.WriteLine([]byte("line 1"))
	rb.WriteLine([]byte("line 2"))
	rb.WriteLine([]byte("line 3"))
	rb.WriteLine([]byte("line 4")) // Overwrites "line 1"

	lines := rb.Read(5)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if string(lines[0]) != "line 2" || string(lines[1]) != "line 3" || string(lines[2]) != "line 4" {
		t.Errorf("incorrect lines read after wrapping: %v", lines)
	}

	lines2 := rb.Read(2)
	if len(lines2) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines2))
	}
	if string(lines2[0]) != "line 3" || string(lines2[1]) != "line 4" {
		t.Errorf("incorrect subset lines read: %v", lines2)
	}
}

func TestLineSplitter(t *testing.T) {
	var lines []string
	ls := newLineSplitter(func(line []byte) {
		lines = append(lines, string(line))
	})

	// Partial write
	ls.Write([]byte("hello "))
	if len(lines) != 0 {
		t.Errorf("expected 0 complete lines, got %d", len(lines))
	}

	ls.Write([]byte("world\nsecond line\nthird "))
	if len(lines) != 2 {
		t.Fatalf("expected 2 complete lines, got %d", len(lines))
	}
	if lines[0] != "hello world" || lines[1] != "second line" {
		t.Errorf("incorrect lines parsed: %q, %q", lines[0], lines[1])
	}

	ls.Write([]byte("line completed\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 complete lines, got %d", len(lines))
	}
	if lines[2] != "third line completed" {
		t.Errorf("incorrect third line parsed: %q", lines[2])
	}
}
