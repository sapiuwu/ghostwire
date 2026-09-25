package util

type AsyncQueue[T any] struct {
	ch           chan T
	done         chan struct{}
	terminalErr  error
	pushed       bool
}

func NewAsyncQueue[T any]() *AsyncQueue[T] {
	return &AsyncQueue[T]{
		ch:   make(chan T, 16),
		done: make(chan struct{}),
	}
}

func (q *AsyncQueue[T]) Size() int {
	return len(q.ch)
}

func (q *AsyncQueue[T]) Push(item T) {
	select {
	case q.ch <- item:
	case <-q.done:
	}
}

func (q *AsyncQueue[T]) RemoveFirst(pred func(T) bool) bool {
	n := len(q.ch)
	items := make([]T, 0, n)
	found := false
	for i := 0; i < n; i++ {
		select {
		case item := <-q.ch:
			if !found && pred(item) {
				found = true
				continue
			}
			items = append(items, item)
		default:
			break
		}
	}
	for _, item := range items {
		q.ch <- item
	}
	return found
}

func (q *AsyncQueue[T]) End(err error) {
	select {
	case <-q.done:
		return
	default:
	}
	q.terminalErr = err
	close(q.done)
}

func (q *AsyncQueue[T]) Next() (T, bool) {
	if len(q.ch) > 0 {
		item := <-q.ch
		return item, true
	}
	<-q.done
	var zero T
	return zero, false
}
