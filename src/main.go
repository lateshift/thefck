package main

import "log"

// main is intentionally tiny: command construction lives in cli.go, while the
// scanner, store, and web server each live in their own focused files.
func main() {
	if err := newRootCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}
