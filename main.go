package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"heckel.io/ntfy/cmd"
	"os"
	"runtime"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cli.AppHelpTemplate += fmt.Sprintf(`
Try 'ntfy COMMAND --help' for more information.

ntfy %s (%s), runtime %s, built at %s
Copyright (C) 2021 Philipp C. Heckel, licensed under Apache License 2.0 & GPLv2
`, version, commit[:7], runtime.Version(), date)

	app := cmd.New()
	app.Version = version

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
