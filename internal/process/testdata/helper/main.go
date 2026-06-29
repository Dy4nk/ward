package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("missing argument")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "sleep":
		for {
			time.Sleep(1 * time.Hour)
		}
	case "exit0":
		os.Exit(0)
	case "exit1":
		os.Exit(1)
	case "ignoreterm":
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM)
		// block until sleep finishes or other signals
		time.Sleep(10 * time.Second)
		os.Exit(2)
	default:
		fmt.Printf("unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
