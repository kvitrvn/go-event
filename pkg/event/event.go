package event

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Listener interface defines an event listener.
type Listener interface {
	Name() string
	Start(data interface{}) bool
	Priority() int
	Process(data interface{}) error
}

// ContextListener can be implemented by listeners that want to receive context cancellation/deadline signals.
type ContextListener interface {
	ProcessWithContext(ctx context.Context, data interface{}) error
}

// EmitError stores contextual information for listener failures.
type EmitError struct {
	EventType    string
	ListenerName string
	Err          error
}

func (e EmitError) Error() string {
	return e.EventType + ": listener " + e.ListenerName + " failed: " + e.Err.Error()
}

func (e EmitError) Unwrap() error {
	return e.Err
}

// EmitResult provides event processing metrics that can be used for observability.
type EmitResult struct {
	EventType          string
	TotalListeners     int
	Started            int
	Skipped            int
	Successful         int
	Failed             int
	Duration           time.Duration
	CancelledByContext bool
}

// Emitter defines the events emitter.
type Emitter struct {
	mu        sync.RWMutex
	listeners map[string][]Listener
}

// NewEmitter returns a new event emitter.
func NewEmitter() *Emitter {
	return &Emitter{listeners: make(map[string][]Listener)}
}

// AddListener adds the given listener to the given event type.
//
// Listeners are inserted by priority using binary search to avoid
// a full sort after each insertion.
func (e *Emitter) AddListener(listener Listener) {
	e.mu.Lock()
	defer e.mu.Unlock()

	eventType := listener.Name()
	listeners := e.listeners[eventType]
	index := sort.Search(len(listeners), func(i int) bool {
		return listeners[i].Priority() > listener.Priority()
	})

	listeners = append(listeners, nil)
	copy(listeners[index+1:], listeners[index:])
	listeners[index] = listener
	e.listeners[eventType] = listeners
}

// AddListeners adds a batch of listeners.
func (e *Emitter) AddListeners(listeners ...Listener) {
	for _, listener := range listeners {
		e.AddListener(listener)
	}
}

// RemoveListener removes one listener by name and priority from the event type.
// Returns true when one listener has been removed.
func (e *Emitter) RemoveListener(eventType string, name string, priority int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	listeners, ok := e.listeners[eventType]
	if !ok {
		return false
	}

	for i, listener := range listeners {
		if listener.Name() == name && listener.Priority() == priority {
			listeners = append(listeners[:i], listeners[i+1:]...)
			if len(listeners) == 0 {
				delete(e.listeners, eventType)
			} else {
				e.listeners[eventType] = listeners
			}
			return true
		}
	}

	return false
}

// ListenerCount returns the number of listeners bound to an event type.
func (e *Emitter) ListenerCount(eventType string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.listeners[eventType])
}

// Emit sends an event with the given data. It keeps backward compatibility
// with existing callers by discarding emitted errors.
func (e *Emitter) Emit(eventType string, data interface{}) {
	_, _ = e.EmitDetailedWithContext(context.Background(), eventType, data)
}

// EmitWithContext sends an event and aggregates processing errors.
func (e *Emitter) EmitWithContext(ctx context.Context, eventType string, data interface{}) error {
	_, err := e.EmitDetailedWithContext(ctx, eventType, data)
	return err
}

// EmitDetailedWithContext sends an event and returns execution metrics plus aggregated errors.
func (e *Emitter) EmitDetailedWithContext(ctx context.Context, eventType string, data interface{}) (EmitResult, error) {
	start := time.Now()
	result := EmitResult{EventType: eventType}

	e.mu.RLock()
	listeners, ok := e.listeners[eventType]
	if !ok {
		e.mu.RUnlock()
		result.Duration = time.Since(start)
		return result, nil
	}
	snapshot := append([]Listener(nil), listeners...)
	e.mu.RUnlock()

	result.TotalListeners = len(snapshot)
	var errs []error

	for _, listener := range snapshot {
		select {
		case <-ctx.Done():
			result.CancelledByContext = true
			err := ctx.Err()
			errs = append(errs, err)
			result.Duration = time.Since(start)
			return result, errors.Join(errs...)
		default:
		}

		if !listener.Start(data) {
			result.Skipped++
			continue
		}

		result.Started++
		if err := callListener(ctx, listener, data); err != nil {
			result.Failed++
			errs = append(errs, EmitError{
				EventType:    eventType,
				ListenerName: listener.Name(),
				Err:          err,
			})
			continue
		}
		result.Successful++
	}

	result.Duration = time.Since(start)
	return result, errors.Join(errs...)
}

// EmitAsyncWithContext sends an event concurrently using a worker pool.
// It is useful for independent listeners with expensive I/O.
func (e *Emitter) EmitAsyncWithContext(ctx context.Context, eventType string, data interface{}, workers int) (EmitResult, error) {
	start := time.Now()
	result := EmitResult{EventType: eventType}

	e.mu.RLock()
	listeners, ok := e.listeners[eventType]
	if !ok {
		e.mu.RUnlock()
		result.Duration = time.Since(start)
		return result, nil
	}
	snapshot := append([]Listener(nil), listeners...)
	e.mu.RUnlock()

	result.TotalListeners = len(snapshot)
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(snapshot) {
		workers = len(snapshot)
	}
	if workers == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	jobs := make(chan Listener)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	worker := func() {
		defer wg.Done()
		for listener := range jobs {
			select {
			case <-ctx.Done():
				mu.Lock()
				result.CancelledByContext = true
				mu.Unlock()
				return
			default:
			}

			if !listener.Start(data) {
				mu.Lock()
				result.Skipped++
				mu.Unlock()
				continue
			}

			err := callListener(ctx, listener, data)
			mu.Lock()
			result.Started++
			if err != nil {
				result.Failed++
				errs = append(errs, EmitError{
					EventType:    eventType,
					ListenerName: listener.Name(),
					Err:          err,
				})
			} else {
				result.Successful++
			}
			mu.Unlock()
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	for _, listener := range snapshot {
		select {
		case <-ctx.Done():
			result.CancelledByContext = true
			break
		case jobs <- listener:
		}
		if result.CancelledByContext {
			break
		}
	}
	close(jobs)
	wg.Wait()

	if result.CancelledByContext {
		errs = append(errs, ctx.Err())
	}
	result.Duration = time.Since(start)
	return result, errors.Join(errs...)
}

func callListener(ctx context.Context, listener Listener, data interface{}) error {
	if contextual, ok := listener.(ContextListener); ok {
		return contextual.ProcessWithContext(ctx, data)
	}
	return listener.Process(data)
}
