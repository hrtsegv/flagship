// Command example is a minimal demonstration of the Flagship Go SDK: it
// connects to a running Flagship server, keeps a boolean flag synced live
// via flagship.Sync, and prints its value whenever it changes -- with no
// polling, and no restart required to observe a change made through the
// control plane's HTTP API.
//
// This is the "First Vertical Slice" from the product requirements:
//
//	Terminal 1: go run ./cmd/flagship
//	Terminal 2: curl ... create a project, environment, and flag
//	Terminal 3: go run ./example -environment=<environmentID>
//	Terminal 2: curl -X PATCH .../flags/{id} -d '{"enabled":true}'
//	Terminal 3 (no restart): the printed value flips to true
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	flagship "github.com/hrtsegv/flagship"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "Flagship server base URL")
	environmentID := flag.String("environment", "", "environment ID to sync (required; returned when you created the environment)")
	flagKey := flag.String("flag", "new-checkout", "flag key to watch")
	flag.Parse()

	if *environmentID == "" {
		log.Fatal("missing required -environment flag")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client := flagship.NewClient(nil)

	go func() {
		err := flagship.Sync(ctx, client, flagship.SyncConfig{
			BaseURL:       *server,
			EnvironmentID: *environmentID,
			OnError: func(err error) {
				log.Printf("sync: %v (retrying)", err)
			},
		})
		if err != nil && ctx.Err() == nil {
			log.Fatalf("sync stopped unexpectedly: %v", err)
		}
	}()

	fmt.Printf("watching %q in environment %s via %s (Ctrl+C to stop)\n", *flagKey, *environmentID, *server)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var last bool
	first := true
	for {
		select {
		case <-ctx.Done():
			fmt.Println("stopped")
			return
		case <-ticker.C:
			current := client.Bool(*flagKey, false)
			if first || current != last {
				fmt.Printf("%s: %v\n", *flagKey, current)
				last, first = current, false
			}
		}
	}
}
