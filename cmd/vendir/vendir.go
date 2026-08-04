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

func isCompletionCmd(c *cobra.Command) bool {
	return c.Name() == "completion" || (c.HasParent() && c.Parent().Name() == "completion")
}
