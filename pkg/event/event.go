package event

import (
	"sort"
)

// Listener interface defines an event listener
type Listener interface {
	Name() string
	Start(data interface{}) bool
	Priority() int
	Process(data interface{}) error
}

// Emitter define the events emitter
type Emitter struct {
	listeners map[string][]Listener
}

// NewEmitter return a new event emitter
func NewEmitter() *Emitter {
	return &Emitter{
		listeners: make(map[string][]Listener),
	}
}

// AddListener add the given listener to the given event type
func (e *Emitter) AddListener(listener Listener) {
	e.listeners[listener.Name()] = append(e.listeners[listener.Name()], listener)
	e.sortListeners(listener.Name())
}

// Emit send an event with the given data
func (e *Emitter) Emit(eventType string, data interface{}) {
	if listeners, ok := e.listeners[eventType]; ok {
		for _, listener := range listeners {
			if listener.Start(data) {
				listener.Process(data)
			}
		}
	}
}

func (e *Emitter) sortListeners(eventType string) {
	if listeners, ok := e.listeners[eventType]; ok {
		sort.Slice(listeners, func(i, j int) bool {
			return listeners[i].Priority() < listeners[j].Priority()
		})
		e.listeners[eventType] = listeners
	}
}
