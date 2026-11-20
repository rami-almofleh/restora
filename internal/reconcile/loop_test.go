package reconcile

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var gvr = schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "things"}

func thing(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Thing",
		"metadata": map[string]any{"name": name, "namespace": "ns"},
	}}
}

func newClient(names ...string) *dynamicfake.FakeDynamicClient {
	objects := make([]runtime.Object, 0, len(names))
	for _, n := range names {
		objects = append(objects, thing(n))
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "ThingList"}, objects...)
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// runUntil runs the loop in the background and stops it when the test is done.
func runUntil(t *testing.T, workers int, client *dynamicfake.FakeDynamicClient, handle Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := RunWorkers(ctx, discard(), client, gvr, "ns", time.Hour, workers, handle); err != nil {
			t.Errorf("RunWorkers: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
}

func TestRunWorkersHandlesSeveralObjectsAtTheSameTime(t *testing.T) {
	var running atomic.Int32
	allRunning := make(chan struct{})
	var once sync.Once

	runUntil(t, 3, newClient("a", "b", "c"), func(ctx context.Context, name string) (time.Duration, error) {
		if running.Add(1) == 3 {
			once.Do(func() { close(allRunning) })
		}
		select {
		case <-allRunning:
		case <-time.After(3 * time.Second):
		}
		return 0, nil
	})

	select {
	case <-allRunning:
	case <-time.After(3 * time.Second):
		t.Fatalf("only %d handlers ran at once, want 3", running.Load())
	}
}

func TestRunHandlesOneObjectAtATime(t *testing.T) {
	var running, peak, done atomic.Int32
	finished := make(chan struct{})

	runUntil(t, 1, newClient("a", "b", "c"), func(ctx context.Context, name string) (time.Duration, error) {
		now := running.Add(1)
		if now > peak.Load() {
			peak.Store(now)
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		if done.Add(1) == 3 {
			close(finished)
		}
		return 0, nil
	})

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatalf("handled %d of 3 objects", done.Load())
	}
	if peak.Load() != 1 {
		t.Errorf("peak = %d, want 1: one worker must never run two handlers at once", peak.Load())
	}
}

func TestRunWorkersNeverHandlesTheSameObjectTwiceAtOnce(t *testing.T) {
	var running, peak, calls atomic.Int32
	enough := make(chan struct{})

	runUntil(t, 4, newClient("only"), func(ctx context.Context, name string) (time.Duration, error) {
		now := running.Add(1)
		if now > peak.Load() {
			peak.Store(now)
		}
		time.Sleep(20 * time.Millisecond)
		running.Add(-1)
		if calls.Add(1) == 3 {
			close(enough)
			return 0, nil
		}
		return 5 * time.Millisecond, nil // asks to be handled again at once
	})

	select {
	case <-enough:
	case <-time.After(3 * time.Second):
		t.Fatalf("handled %d times", calls.Load())
	}
	if peak.Load() != 1 {
		t.Errorf("peak = %d, want 1", peak.Load())
	}
}
