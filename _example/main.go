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

	r.Use(valv.Handler) // we can remove this with the base context stuff..
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("sup"))
	})

	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {

		// hmm.. if <-shutdown.ShutdownCh is closed.. then, we shouldn't allow anymore processing.
		// tylerb/graceful will already release the port though and let requests finish.
		// this is more important for background workers etc. streaming, etc. to tell them
		// to finish up.
		// perhaps shutdown should have ForceShutdown() which calls the cancel on the base context..
		//
		// --
		//
		// so, we sorta need a base context
		// chi.WithBaseContext(http.Handler, context.Context) http.handler

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
			// and respond with what we have so far. How a shutdown is handled it entirely
			// up to the developer, but the signal will hit.
			//
			// depends on what can be preempetted, and what cannot.
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
				// shutdown.Context(ctx).Add(1)
				// defer shutdown.Context(ctx).Done()

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
		BeforeShutdown: func() bool {
			fmt.Println("shutting down..")
			valv.Shutdown()
			return true
		},
	}
	srv.ListenAndServe()
}
