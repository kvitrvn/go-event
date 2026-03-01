package event

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

var (
	errBoom = errors.New("boom")
	errBang = errors.New("bang")
)

type testListener struct {
	name     string
	priority int
	startOK  bool
	callLog  *[]int
	err      error
}

func (l testListener) Name() string                { return l.name }
func (l testListener) Priority() int               { return l.priority }
func (l testListener) Start(data interface{}) bool { return l.startOK }
func (l testListener) Process(data interface{}) error {
	if l.callLog != nil {
		*l.callLog = append(*l.callLog, l.priority)
	}
	return l.err
}

func TestAddListenerKeepsAscendingPriority(t *testing.T) {
	e := NewEmitter()
	log := make([]int, 0, 3)
	e.AddListeners(
		testListener{name: "evt", priority: 20, startOK: true, callLog: &log},
		testListener{name: "evt", priority: 1, startOK: true, callLog: &log},
		testListener{name: "evt", priority: 5, startOK: true, callLog: &log},
	)

	e.Emit("evt", nil)

	expected := []int{1, 5, 20}
	if !slices.Equal(log, expected) {
		t.Fatalf("expected call order %v, got %v", expected, log)
	}
}

func TestEmitWithContextCollectsErrors(t *testing.T) {
	e := NewEmitter()
	e.AddListeners(
		testListener{name: "evt", priority: 0, startOK: true, err: errBoom},
		testListener{name: "evt", priority: 1, startOK: true, err: errBang},
	)

	err := e.EmitWithContext(context.Background(), "evt", nil)
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	if !errors.Is(err, errBoom) || !errors.Is(err, errBang) {
		t.Fatalf("expected joined error to include both causes, got %q", err)
	}
	if !strings.Contains(err.Error(), "listener evt failed") {
		t.Fatalf("expected contextual listener details, got %q", err)
	}
}

func TestRemoveListener(t *testing.T) {
	e := NewEmitter()
	e.AddListeners(
		testListener{name: "evt", priority: 0, startOK: true},
		testListener{name: "evt", priority: 1, startOK: true},
	)

	removed := e.RemoveListener("evt", "evt", 0)
	if !removed {
		t.Fatal("expected listener removal to succeed")
	}
	if got := e.ListenerCount("evt"); got != 1 {
		t.Fatalf("expected 1 listener after removal, got %d", got)
	}
}
