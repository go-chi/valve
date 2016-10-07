package valve

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

var (
	ValveCtxKey     = &contextKey{"ValveContext"}
	ErrTimedout     = errors.New("valve: shutdown timed out")
	ErrShuttingdown = errors.New("valve: shutdown in progress")
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation. This technique
// for defining context keys was copied from Go 1.7's new use of context in net/http.
type contextKey struct {
	name string
}

func (k *contextKey) String() string {
	return "valve context value " + k.name
}

type Valve struct {
	stopCh chan struct{}
	wg     sync.WaitGroup

	mu sync.Mutex
}

type Lever interface {
	Stop() <-chan struct{}
	Add(delta int) error
	Done()
	Open() error
	Close()
}

func New() *Valve {
	return &Valve{
		stopCh: make(chan struct{}, 0),
	}
}

func (v *Valve) Handler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ValveCtxKey, Lever(v))
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func (v *Valve) ShutdownHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		valv := Context(r.Context())
		valv.Open()
		defer valv.Close()

		select {
		// Shutdown in progress - don't accept new requests
		case <-valv.Stop():
			http.Error(w, ErrShuttingdown.Error(), http.StatusServiceUnavailable)

		default:
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}

// Shutdown will signal to the context to stop all processing, and will
// give a grace period of `timeout` duration. If `timeout` is 0 then it will
// wait indefinitely until all valves are closed.
func (v *Valve) Shutdown(timeout time.Duration) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	close(v.stopCh)

	if timeout == 0 {
		v.wg.Wait()
	} else {
		tc := make(chan struct{})
		go func() {
			defer close(tc)
			v.wg.Wait()
		}()
		select {
		case <-tc:
			return nil
		case <-time.After(timeout):
			return ErrTimedout
		}
	}

	return nil
}

func (v *Valve) Stop() <-chan struct{} {
	return v.stopCh
}

func (v *Valve) Add(delta int) error {
	select {
	case <-v.stopCh:
		return ErrShuttingdown
	default:
		v.wg.Add(delta)
		return nil
	}
}

func (v *Valve) Done() {
	v.wg.Done()
}

func (v *Valve) Open() error {
	return v.Add(1)
}

func (v *Valve) Close() {
	v.Done()
}

func Context(ctx context.Context) Lever {
	valveCtx, ok := ctx.Value(ValveCtxKey).(Lever)
	if !ok {
		panic("valve: ValveCtxKey has not been set on the context.")
	}
	return valveCtx
}
