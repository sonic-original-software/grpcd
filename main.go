//revive:disable:package-comments
package main

import (
	"os"

	"git.sonicoriginal.software/grpcd/internal/cli"
)

func main() { os.Exit(cli.Run()) }
