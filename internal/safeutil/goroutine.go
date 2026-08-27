// Package safeutil provides panic-safe goroutine wrappers and graceful shutdown.
package safeutil

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"haovpn/internal/logger"
)

// GoSafe runs fn in a goroutine, recovering panics and logging stack traces.
func GoSafe(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine %s panic: %v", name, r)
			}
		}()
		fn()
	}()
}

// GoSafeCtx runs fn until ctx is cancelled or fn returns; recovers panics.
func GoSafeCtx(ctx context.Context, name string, fn func(context.Context)) {
	GoSafe(name, func() {
		fn(ctx)
	})
}

// Shutdown coordinates graceful shutdown across components.
type Shutdown struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	done   atomic.Bool
}

// NewShutdown creates a shutdown coordinator listening for SIGINT/SIGTERM.
func NewShutdown() *Shutdown {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Shutdown{ctx: ctx, cancel: cancel}
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	GoSafe("signal-handler", func() {
		select {
		case sig := <-ch:
			logger.Info("received signal %s, initiating graceful shutdown", sig)
			s.cancel()
		case <-ctx.Done():
		}
		signal.Stop(ch)
	})
	return s
}

// Context returns the root shutdown context.
func (s *Shutdown) Context() context.Context { return s.ctx }

// Go runs fn in a tracked goroutine.
func (s *Shutdown) Go(name string, fn func(context.Context)) {
	s.wg.Add(1)
	GoSafe(name, func() {
		defer s.wg.Done()
		fn(s.ctx)
	})
}

// Cancel triggers shutdown.
func (s *Shutdown) Cancel() { s.cancel() }

// Wait blocks until all tracked goroutines finish or timeout expires.
func (s *Shutdown) Wait(timeout time.Duration) {
	s.once.Do(func() {
		s.done.Store(true)
	})
	done := make(chan struct{})
	GoSafe("shutdown-wait", func() {
		s.wg.Wait()
		close(done)
	})
	select {
	case <-done:
		logger.Info("all goroutines stopped")
	case <-time.After(timeout):
		logger.Warn("shutdown timeout after %s, some goroutines may still be running", timeout)
	}
}

// Done reports whether shutdown has been initiated.
func (s *Shutdown) Done() bool { return s.done.Load() }
