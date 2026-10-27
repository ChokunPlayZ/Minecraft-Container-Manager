package checkcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCacheCoalescesFailuresAndRefreshesEachPass(t *testing.T) {
	var calls atomic.Int32
	fetch := func() (string, error) { calls.Add(1); return "", errors.New("upstream failed") }
	ctx := NewContext(context.Background())
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := Do(ctx, "same-request", fetch); err == nil {
				t.Error("failure was hidden")
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("same pass called upstream %d times", calls.Load())
	}
	_, _ = Do(NewContext(context.Background()), "same-request", fetch)
	if calls.Load() != 2 {
		t.Fatal("new pass reused old failure")
	}
}
