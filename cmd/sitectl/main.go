// Command sitectl validates and tests site configs.
package main

import (
	"os"

	"go-wordpress/internal/sitectl"
)

func main() {
	os.Exit(sitectl.CLI{Stdout: os.Stdout, Stderr: os.Stderr}.Run(os.Args[1:]))
}
