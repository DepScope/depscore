package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageCheckCommand(t *testing.T) {
	stub := &testFetcher{}
	cmd := &cobra.Command{}
	cmd.Flags().String("ecosystem", "python", "")
	cmd.Flags().String("profile", "hobby", "")
	var stdout bytes.Buffer
	err := runPackageCheckWith(&stdout, stub, cmd, []string{"requests==2.31.0"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "requests")
}
