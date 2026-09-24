package bench

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

type coordinatedTiming struct {
	Target      time.Time
	SpinLead    time.Duration
	LockThreads bool
}

func (t coordinatedTiming) run(ctx context.Context, start <-chan struct{}, ready *sync.WaitGroup, fn func()) {
	if t.LockThreads {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	ready.Done()
	<-start
	waitUntil(ctx, t.Target, t.SpinLead)
	fn()
}

func (t coordinatedTiming) release(ctx context.Context, ready *sync.WaitGroup, start chan struct{}) {
	ready.Wait()
	waitUntil(ctx, t.Target.Add(-t.SpinLead), 0)
	close(start)
}

func randomizedOrder(n int) []int {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []int{0}
	}
	return rand.Perm(n)
}

func waitUntil(ctx context.Context, target time.Time, spinLead time.Duration) {
	if target.IsZero() {
		return
	}
	if spinLead < 0 {
		spinLead = 0
	}
	sleepTarget := target.Add(-spinLead)
	delay := time.Until(sleepTarget)
	if delay <= 0 {
		spinUntil(ctx, target)
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
		spinUntil(ctx, target)
	}
}

func spinUntil(ctx context.Context, target time.Time) {
	for {
		if ctx.Err() != nil {
			return
		}
		remaining := time.Until(target)
		if remaining <= 0 {
			return
		}
		if remaining > 100*time.Microsecond {
			runtime.Gosched()
		}
	}
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
