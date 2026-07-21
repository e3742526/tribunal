package main

import (
	"errors"
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
		code := app.ExitInvalidArguments
		var exit *app.ExitError
		if errors.As(err, &exit) {
			code = exit.Code
		}
		os.Exit(code)
	}
}
