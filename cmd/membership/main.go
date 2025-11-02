// cmd/membership/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gavinmcnair/peerplex/internal/membership"
)

func main() {
	// Flags – could use viper/cobra, but explicit for MVP
	var id string
	var bind string
	var seeds string
	flag.StringVar(&id, "id", "", "Unique node ID (required)")
	flag.StringVar(&bind, "bind", ":9700", "Address to bind (host:port)")
	flag.StringVar(&seeds, "seeds", "", "Comma-separated bootstrap addresses")

	flag.Parse()
	if id == "" {
		fmt.Println("id is required")
		os.Exit(1)
	}

	seedList := []string{}
	if seeds != "" {
		seedList = strings.Split(seeds, ",")
	}

	// Config
	cfg := &membership.Config{
		NodeID:         id,
		BindAddr:       bind,
		Seeds:          seedList,
		GossipInterval: 1 * time.Second,
		ProbeTimeout:   500 * time.Millisecond,
		ProbeRetries:   3,
	}

	// Start membership
	mem, err := membership.NewMembership(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "membership: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trap Ctrl+C
	cdone := make(chan os.Signal, 1)
	signal.Notify(cdone, os.Interrupt)

	go func() {
		<-cdone
		fmt.Println("\nShutting down...")
		cancel()
		mem.Stop()
		os.Exit(0)
	}()

	// Start membership (blocks only enough to join/gossip)
	go func() {
		if err := mem.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Print live peers every second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			fmt.Printf("[%s] Live peers: ", id)
			for _, p := range mem.Live() {
				fmt.Printf("%s(%s) ", p.ID, p.Addr)
			}
			fmt.Println()
		}
	}
}

