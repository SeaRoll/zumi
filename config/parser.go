package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Regex to find all occurrences of ${VAR:fallback}.
var re = regexp.MustCompile(`\$\{(\w+):([^}]+)\}`)

// FromYAML creates a Config instance from a YAML string.
func FromYAML[T ZumiConfig](data string) (T, error) {
	content, err := LoadConfig[T](data)
	if err != nil {
		return content, fmt.Errorf("failed to load config: %w", err)
	}

	return content, nil
}

// expandEnvVars processes a string to replace placeholders like ${VAR:default}
// with the corresponding environment variable's value or the default.
func expandEnvVars(s string) string {
	// Find all matches in the input string
	matches := re.FindAllStringSubmatch(s, -1)

	// If no matches are found, return the original string
	if matches == nil {
		return s
	}

	// Iterate over all found matches
	for _, match := range matches {
		// The full match is match[0], e.g., "${DATABASE_HOST:localhost}"
		// The variable name is match[1], e.g., "DATABASE_HOST"
		// The fallback value is match[2], e.g., "localhost"
		fullMatch := match[0]
		envVar := match[1]
		fallback := match[2]

		// Get the value from the environment
		value := os.Getenv(envVar)
		if value == "" {
			// If the environment variable is not set, use the fallback
			value = fallback
		}

		// Replace the placeholder with the resolved value
		s = strings.ReplaceAll(s, fullMatch, value)
	}

	return s
}

// processNode recursively traverses the YAML node tree and expands environment variables in string values.
func processNode(node *yaml.Node) error {
	// We only care about scalar nodes (string, int, bool, etc.)
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		// Expand any environment variables in the string value.
		originalValue := node.Value
		node.Value = expandEnvVars(node.Value)

		// If the value changed, clear the tag. This forces the decoder to
		// re-infer the type. For example, a string "${PORT:8080}" becomes "8080",
		// and clearing the tag allows it to be decoded as an integer.
		if node.Value != originalValue {
			node.Tag = ""
		}
	}

	// Recursively process the content of the node (for maps and sequences)
	for _, child := range node.Content {
		err := processNode(child)
		if err != nil {
			return err
		}
	}

	return nil
}

// LoadConfig reads, parses, and processes the YAML configuration.
func LoadConfig[T any](data string) (T, error) {
	var cfg T

	// Use a yaml.Node to get a raw representation of the YAML structure
	var root yaml.Node

	err := yaml.Unmarshal([]byte(data), &root)
	if err != nil {
		return cfg, fmt.Errorf("failed to unmarshal yaml: %w", err)
	}

	// Process the node tree to expand environment variables
	err = processNode(&root)
	if err != nil {
		return cfg, fmt.Errorf("failed to process yaml nodes: %w", err)
	}

	// Marshal the processed node tree back to YAML bytes
	processedData, err := yaml.Marshal(&root)
	if err != nil {
		return cfg, fmt.Errorf("failed to marshal processed yaml: %w", err)
	}

	// Unmarshal the final YAML bytes into our Config struct
	err = yaml.Unmarshal(processedData, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to unmarshal processed yaml into struct: %w", err)
	}

	return cfg, nil
}
