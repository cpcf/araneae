package crawl

import (
	"context"
	"math"
	"sync"
	"time"
)

type requestGate interface {
	Wait(context.Context) error
}

type noopRequestGate struct{}

func (noopRequestGate) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

type intervalRequestGate struct {
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

func newRequestGate(maxRequestsPerSecond float64) requestGate {
	if maxRequestsPerSecond <= 0 || math.IsNaN(maxRequestsPerSecond) || math.IsInf(maxRequestsPerSecond, 0) {
		return noopRequestGate{}
	}
	interval := time.Duration(float64(time.Second) / maxRequestsPerSecond)
	if interval <= 0 {
		return noopRequestGate{}
	}
	return &intervalRequestGate{interval: interval}
}

func (g *intervalRequestGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	now := time.Now()
	waitUntil := now
	if g.next.After(now) {
		waitUntil = g.next
	}
	g.next = waitUntil.Add(g.interval)
	delay := time.Until(waitUntil)
	g.mu.Unlock()

	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
