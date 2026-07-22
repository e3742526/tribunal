package main

import (
	"fmt"
	"os"

	"github.com/e3742526/tribunal/internal/cli"
	"github.com/e3742526/tribunal/internal/tribunal/app"
)

func main() {
	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		if err.Error() != "" {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(app.ExitCodeFor(err))
	}
}
