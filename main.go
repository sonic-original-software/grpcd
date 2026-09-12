//revive:disable:package-comments
package main

import (
	"os"

	"github.com/grpcd/server/internal/cli"
)

func main() { os.Exit(cli.Run()) }
