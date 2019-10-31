package waitpool

import (
	"context"
	"sync"
)

type WaitWorkerPool struct {
	wg *sync.WaitGroup
	queue chan func()
	discardTasks bool
	mutex sync.RWMutex
}

func (w *WaitWorkerPool) close() {
	w.mutex.Lock()
	w.discardTasks = true
	close(w.queue)
	w.mutex.Unlock()
	// drain the remaining tasks in the queue and end them
	for {
		select {
		case _, ok := <-w.queue:
			if !ok {
				return
			}
			w.wg.Done()
		}
	}
}

func (w *WaitWorkerPool) AddTask(fn func()) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	if w.discardTasks {
		return
	}
	w.wg.Add(1)
	w.queue <- fn
}


func Do(workers int, fn func(pool *WaitWorkerPool)) {
	ctx, cancel := context.WithCancel(context.Background())
	DoWithContext(ctx, workers, fn)
	cancel()
}

func DoWithContext(ctx context.Context, workers int, fn func(pool *WaitWorkerPool)) {
	pool := &WaitWorkerPool{wg:&sync.WaitGroup{}, queue: make(chan func(), workers)}
	for w := 0; w < workers; w++ {
		go func() {
			for {
				select {
				case task, ok := <-pool.queue:
					if !ok {
						continue
					}
					task()
					pool.wg.Done()
				case <-ctx.Done():
					return
				}
			}
		} ()
	}

	done := make(chan struct{}, 1)
	go func() {
		fn(pool)
		pool.wg.Wait()
		done <- struct{}{}
	} ()

	// wait for the tasks to finish or the cancel
	drained := false
	select {
	case <-done:
		drained = true
	case <-ctx.Done():
	}

	pool.close()
	if !drained {
		<- done
	}
	close(done)
}
