// Package main is the entry point for the Secure FTP application.
package main

import (
	"fmt"
	"os"

	"secure-ftp/internal/app"
)

func main() {
	application, err := app.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	application.Run()
}
