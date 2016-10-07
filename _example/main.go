package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/goware/valve"
	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
	"github.com/tylerb/graceful"
)

func main() {

	valv := valve.New()

	//-------

	r := chi.NewRouter()

	r.Use(valv.Handler) // TODO: we can remove this with the base context stuff..
	// something like..chi.WithBaseContext(http.Handler, context.Context) http.handler

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sup"))
	})

	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {

		valve.Context(r.Context()).Open()
		defer valve.Context(r.Context()).Close()

		select {
		case <-valve.Context(r.Context()).Stop():
			fmt.Println("valve is closed. finish up..")

		case <-time.After(5 * time.Second):
			// The above channel simulates some hard work.
			// We want this handler to complete successfully during a shutdown signal,
			// so consider the work here as some background routine to fetch a long running
			// search query to find as many results as possible, but, instead we cut it short
			// and respond with what we have so far. How a shutdown is handled is entirely
			// up to the developer, as some code blocks are preemptable, and others are not.
			time.Sleep(5 * time.Second)
		}

		w.Write([]byte(fmt.Sprintf("all done.\n")))
	})

	// Example of a long running background worker thing..
	ctx := context.WithValue(context.Background(), valve.ValveCtxKey, valve.Lever(valv))
	go func(ctx context.Context) {
		for {
			<-time.After(1 * time.Second)

			func() {
				valve.Context(ctx).Open()
				defer valve.Context(ctx).Close()

				// actual code doing stuff..
				fmt.Println("tick..")
				time.Sleep(2 * time.Second)
				// end-logic

				// signal control..
				select {
				case <-valve.Context(ctx).Stop():
					fmt.Println("valve is closed")
					return

				case <-ctx.Done():
					fmt.Println("context is cancelled, go home.")
					return
				default:
				}
			}()

		}
	}(ctx)

	// -------

	// c := make(chan os.Signal, 1)
	// signal.Notify(c, os.Interrupt)
	// go func() {
	// 	for sig := range c {
	// 		// sig is a ^C, handle it
	// 		valv.Shutdown()
	// 		os.Exit(1)
	// 	}
	// }()
	// http.ListenAndServe(":3333", r)

	srv := &graceful.Server{
		Timeout: 20 * time.Second,
		Server:  &http.Server{Addr: ":3333", Handler: r},
	}
	srv.BeforeShutdown = func() bool {
		fmt.Println("shutting down..")
		err := valv.Shutdown(srv.Timeout)
		if err != nil {
			fmt.Println("Shutdown error -", err)
		}
		// the app code has stopped here now, and so this would be a good place
		// to close up any db and other service connections, etc.
		return true
	}
	srv.ListenAndServe()
}
