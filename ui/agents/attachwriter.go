package agents

import "sync"

// attachWriter sends an attach's writes in order on its own goroutine, so a
// holder that stops reading can't freeze Update. The queue is unbounded:
// dropping or blocking on a full one would lose keys or freeze the TUI again.
type attachWriter struct {
	mu    sync.Mutex
	queue []func(AttachStream) error
	// dead: the writer stopped (closed or failed); later writes are dropped.
	dead bool
	err  error
	wake chan struct{}
	stop chan struct{}
	// pending counts queued writes not yet done, for tests to wait on.
	pending sync.WaitGroup
}

func startAttachWriter(stream AttachStream) *attachWriter {
	w := &attachWriter{wake: make(chan struct{}, 1), stop: make(chan struct{})}
	go w.run(stream)
	return w
}

func (w *attachWriter) enqueue(write func(AttachStream) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dead {
		return
	}
	w.pending.Add(1)
	w.queue = append(w.queue, write)
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// close stops the writer; the caller closes the stream, which unblocks a
// write in progress.
func (w *attachWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.dead {
		close(w.stop)
	}
	w.dropQueue()
}

// failure is the write error that ended the attach, if any.
func (w *attachWriter) failure() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

func (w *attachWriter) run(stream AttachStream) {
	for {
		w.mu.Lock()
		if w.dead {
			w.mu.Unlock()
			return
		}
		if len(w.queue) == 0 {
			w.mu.Unlock()
			select {
			case <-w.wake:
				continue
			case <-w.stop:
				return
			}
		}
		write := w.queue[0]
		w.queue = w.queue[1:]
		w.mu.Unlock()

		err := write(stream)
		w.pending.Done()
		if err != nil {
			// Closing the stream ends its read too, so the attach ends the
			// way a read error ends it.
			w.mu.Lock()
			w.err = err
			w.dropQueue()
			w.mu.Unlock()
			_ = stream.Close()
			return
		}
	}
}

// dropQueue marks the writer dead and discards what's queued. w.mu is held.
func (w *attachWriter) dropQueue() {
	w.dead = true
	for range w.queue {
		w.pending.Done()
	}
	w.queue = nil
}
