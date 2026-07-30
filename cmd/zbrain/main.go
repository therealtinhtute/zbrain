package main

import (
	"fmt"
	"os"

	"github.com/therealtinhtute/zbrain/internal/cli"
)

func main() {
	app, err := cli.New()
	if err == nil {
		err = app.Run(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
