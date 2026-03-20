package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// exitError is returned by RunE functions to signal a specific exit code
// without triggering cobra's error printing.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

var rootCmd = &cobra.Command{
	Use:           "depscope",
	Short:         "Supply chain reputation scoring for your dependencies",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
