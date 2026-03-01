package event

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

type contextAwareListener struct {
	testListener
	ctxHit *atomic.Bool
}

func (l contextAwareListener) ProcessWithContext(ctx context.Context, data interface{}) error {
	l.ctxHit.Store(true)
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

func TestEmitDetailedWithContextResult(t *testing.T) {
	e := NewEmitter()
	e.AddListeners(
		testListener{name: "evt", priority: 0, startOK: false},
		testListener{name: "evt", priority: 1, startOK: true},
		testListener{name: "evt", priority: 2, startOK: true, err: errBoom},
	)

	result, err := e.EmitDetailedWithContext(context.Background(), "evt", nil)
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("expected result error to include boom, got %v", err)
	}
	if result.TotalListeners != 3 || result.Skipped != 1 || result.Started != 2 || result.Successful != 1 || result.Failed != 1 {
		t.Fatalf("unexpected result counters: %+v", result)
	}
	if result.Duration <= 0 {
		t.Fatalf("expected positive duration, got %s", result.Duration)
	}
}

func TestEmitDetailedWithContextUsesContextAwareListener(t *testing.T) {
	e := NewEmitter()
	hit := &atomic.Bool{}
	e.AddListener(contextAwareListener{testListener: testListener{name: "evt", startOK: true}, ctxHit: hit})

	_, err := e.EmitDetailedWithContext(context.Background(), "evt", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !hit.Load() {
		t.Fatal("expected ProcessWithContext to be used")
	}
}

func TestEmitAsyncWithContext(t *testing.T) {
	e := NewEmitter()
	e.AddListeners(
		testListener{name: "evt", priority: 0, startOK: true},
		testListener{name: "evt", priority: 1, startOK: true, err: errBoom},
		testListener{name: "evt", priority: 2, startOK: false},
	)

	result, err := e.EmitAsyncWithContext(context.Background(), "evt", nil, 2)
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("expected async error to include boom, got %v", err)
	}
	if result.TotalListeners != 3 || result.Started != 2 || result.Skipped != 1 || result.Successful != 1 || result.Failed != 1 {
		t.Fatalf("unexpected async result counters: %+v", result)
	}
}

func TestEmitAsyncWithContextCancelled(t *testing.T) {
	e := NewEmitter()
	e.AddListener(testListener{name: "evt", priority: 0, startOK: true})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := e.EmitAsyncWithContext(ctx, "evt", nil, 1)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if !result.CancelledByContext {
		t.Fatalf("expected cancelled result, got %+v", result)
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

func TestEmitWithContextStopsOnCancellation(t *testing.T) {
	e := NewEmitter()
	e.AddListener(testListener{name: "evt", priority: 0, startOK: true, err: nil})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	result, err := e.EmitDetailedWithContext(ctx, "evt", nil)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !result.CancelledByContext {
		t.Fatalf("expected cancellation flag, got %+v", result)
	}
}
