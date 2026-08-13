// Copyright 2024 The Carvel Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"log"
	"math/rand"
	"os"
	"time"

	"carvel.dev/vendir/pkg/vendir/cmd"
	uierrs "github.com/cppforlife/go-cli-ui/errors"
	"github.com/cppforlife/go-cli-ui/ui"
	"github.com/spf13/cobra"
)

func main() {
	rand.New(rand.NewSource(time.Now().UTC().UnixNano()))

	log.SetOutput(io.Discard)

	// TODO logs
	// TODO log flags used

	confUI := ui.NewConfUI(ui.NewNoopLogger())
	defer confUI.Flush()

	command := cmd.NewDefaultVendirCmd(confUI)

	executedCmd, err := command.ExecuteC()
	if err != nil {
		confUI.ErrorLinef("vendir: Error: %v", uierrs.NewMultiLineError(err))
		os.Exit(1)
	}

	// The "completion" command (and its shell subcommands) prints a script
	// that is meant to be sourced/eval'd directly, so it must not be
	// followed by any extra output.
	if !isCompletionCmd(executedCmd) {
		confUI.PrintLinef("Succeeded")
	}
}

// isCompletionCmd reports whether the executed command is the "completion"
// command (or one of its shell subcommands), or Cobra's hidden "__complete"
// / "__completeNoDesc" command used to serve live shell completions. Output
// from any of these must not be followed by extra text.
func isCompletionCmd(c *cobra.Command) bool {
	for ; c != nil; c = c.Parent() {
		switch c.Name() {
		case "completion", cobra.ShellCompRequestCmd:
			return true
		}
	}
	return false
}
