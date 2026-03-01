package event

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// Listener interface defines an event listener.
type Listener interface {
	Name() string
	Start(data interface{}) bool
	Priority() int
	Process(data interface{}) error
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
	_ = e.EmitWithContext(context.Background(), eventType, data)
}

// EmitWithContext sends an event and aggregates processing errors.
func (e *Emitter) EmitWithContext(ctx context.Context, eventType string, data interface{}) error {
	e.mu.RLock()
	listeners, ok := e.listeners[eventType]
	if !ok {
		e.mu.RUnlock()
		return nil
	}
	snapshot := append([]Listener(nil), listeners...)
	e.mu.RUnlock()

	var errs []error
	for _, listener := range snapshot {
		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			return errors.Join(errs...)
		default:
		}

		if !listener.Start(data) {
			continue
		}
		if err := listener.Process(data); err != nil {
			errs = append(errs, EmitError{
				EventType:    eventType,
				ListenerName: listener.Name(),
				Err:          err,
			})
		}
	}

	return errors.Join(errs...)
}
