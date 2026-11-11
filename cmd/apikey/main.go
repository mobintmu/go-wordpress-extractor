// Command apikey generates, lists, and revokes client API keys.
package main

import (
	"os"

	"go-wordpress/internal/apikey"
)

func main() {
	os.Exit(apikey.CLI{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}.Run(os.Args[1:]))
}
