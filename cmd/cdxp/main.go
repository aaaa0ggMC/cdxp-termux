// Command cdxp launches the Codex CLI with a named JSON profile.
package main

import (
	"os"

	"github.com/aaaa0ggMC/cdxp-termux/internal/cdxp"
)

func main() {
	os.Exit(cdxp.New().Run(os.Args[1:]))
}
