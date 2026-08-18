// Copyright 2024 The Carvel Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIsCompletionCmd(t *testing.T) {
	root := &cobra.Command{Use: "vendir"}
	completion := &cobra.Command{Use: "completion"}
	completionBash := &cobra.Command{Use: "bash"}
	shellComplete := &cobra.Command{Use: cobra.ShellCompRequestCmd}
	shellCompleteNoDesc := &cobra.Command{Use: cobra.ShellCompNoDescRequestCmd}
	sync := &cobra.Command{Use: "sync"}

	root.AddCommand(completion, shellComplete, shellCompleteNoDesc, sync)
	completion.AddCommand(completionBash)

	cases := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"nil command", nil, false},
		{"completion command itself", completion, true},
		{"completion shell subcommand", completionBash, true},
		{"hidden __complete command", shellComplete, true},
		{"hidden __completeNoDesc command", shellCompleteNoDesc, true},
		{"unrelated command", sync, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCompletionCmd(c.cmd); got != c.want {
				t.Errorf("isCompletionCmd(%v) = %v, want %v", c.cmd, got, c.want)
			}
		})
	}
}
