package cmd

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

func isStructuredOutput() bool {
	return outputFormat == "json" || outputFormat == "yaml"
}

func printOutput(data any) error {
	switch outputFormat {
	case "json":
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(out))
	case "yaml":
		out, err := yaml.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(out))
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
	return nil
}
