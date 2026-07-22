package main

import (
	"fmt"
	"os"
	"time"

	"github.com/e3742526/tribunal/internal/architecture"
)

func main() {
	if err := architecture.Validate(".", time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("architecture registry verified")
}
