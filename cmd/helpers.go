package cmd

import (
	"fmt"
	"io"
	"os"
)

// readValueFromArgs reads a value from command args or stdin.
// If args has a value and it's "-", reads from stdin.
// If args has no value, reads from stdin.
// Otherwise returns the value from args.
func readValueFromArgs(args []string, argIndex int) (string, error) {
	if len(args) > argIndex {
		if args[argIndex] == "-" {
			return readStdin()
		}
		return args[argIndex], nil
	}
	return readStdin()
}

func readStdin() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read from stdin: %w", err)
	}
	return string(data), nil
}
