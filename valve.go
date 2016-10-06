package valve

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

var (
	ValveCtxKey = &contextKey{"ValveContext"}
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
}

type Lever interface {
	Stop() <-chan struct{}
	Add(delta int)
	Done()
	Open()
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

// TODO: accept max time argument..
func (v *Valve) Shutdown() error {
	fmt.Println("shutdown now.") // TODO: remove comment..
	// maybe take other logger?
	close(v.stopCh)
	v.wg.Wait() // TODO: timeout after max time
	return nil
}

func (v *Valve) Stop() <-chan struct{} {
	return v.stopCh
}

func (v *Valve) Add(delta int) {
	select {
	case <-v.stopCh:
	default:
		v.wg.Add(delta)
	}
}

func (v *Valve) Done() {
	v.wg.Done()
}

func (v *Valve) Open() {
	v.Add(1)
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
