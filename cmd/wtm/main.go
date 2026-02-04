package main

import (
	"fmt"
	"os"

	"github.com/aayushgautam/wtm/internal/build"
	"github.com/aayushgautam/wtm/internal/sync"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wtm <sync|push|version> [options]")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "sync":
		if err := sync.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	case "push":
		if err := sync.Push(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	case "version":
		fmt.Println(build.Version)
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		os.Exit(2)
	}
}
