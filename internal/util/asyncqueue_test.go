package util_test

import (
	"errors"
	"testing"

	"ghostwire/internal/util"
)

func TestAsyncQueueFIFO(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	q.Push(1)
	q.Push(2)
	q.Push(3)

	for _, want := range []int{1, 2, 3} {
		got, ok := q.Next()
		if !ok {
			t.Fatal("Next returned false")
		}
		if got != want {
			t.Errorf("expected %d, got %d", want, got)
		}
	}
}

func TestAsyncQueueEnd(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	q.Push(1)
	q.End(nil)

	got, ok := q.Next()
	if !ok {
		t.Fatal("Next returned false for buffered item")
	}
	if got != 1 {
		t.Errorf("expected 1, got %d", got)
	}

	_, ok = q.Next()
	if ok {
		t.Error("Next should return false after End")
	}
}

func TestAsyncQueueEndWithError(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	testErr := errors.New("test error")
	q.End(testErr)

	_, ok := q.Next()
	if ok {
		t.Error("Next should return false after End with error")
	}
}

func TestAsyncQueueRemoveFirst(t *testing.T) {
	q := util.NewAsyncQueue[string]()
	q.Push("a")
	q.Push("b")
	q.Push("c")

	found := q.RemoveFirst(func(s string) bool { return s == "b" })
	if !found {
		t.Error("RemoveFirst should find 'b'")
	}

	got, _ := q.Next()
	if got != "a" {
		t.Errorf("expected 'a', got %q", got)
	}
	got, _ = q.Next()
	if got != "c" {
		t.Errorf("expected 'c', got %q", got)
	}
}

func TestAsyncQueueRemoveFirstNotFound(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	q.Push(1)
	q.Push(2)

	found := q.RemoveFirst(func(i int) bool { return i == 99 })
	if found {
		t.Error("RemoveFirst should not find 99")
	}
}

func TestAsyncQueueSize(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}
	q.Push(1)
	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}
	q.Push(2)
	if q.Size() != 2 {
		t.Errorf("expected size 2, got %d", q.Size())
	}
}

func TestAsyncQueueEndIdempotent(t *testing.T) {
	q := util.NewAsyncQueue[int]()
	q.End(nil)
	q.End(nil) // should not panic
}
