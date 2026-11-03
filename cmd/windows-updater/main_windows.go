package main

import (
	"context"
	"fmt"
	"os"

	"transcoder/update/managed"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--recover" {
		fmt.Fprintln(os.Stderr, "usage: transcoderd-updater --recover")
		os.Exit(2)
	}
	if err := managed.RunHelper(context.Background()); err != nil {
		managed.WriteLog(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
