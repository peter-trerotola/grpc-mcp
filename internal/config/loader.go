package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envVarRegex matches ${VAR_NAME} patterns for environment variable expansion.
var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads and parses a configuration file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from YAML bytes with environment variable expansion.
func Parse(data []byte) (*Config, error) {
	// Expand environment variables before parsing
	expanded := expandEnvVars(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &config, nil
}

// expandEnvVars replaces ${VAR_NAME} patterns with environment variable values.
// If the variable is not set, it remains as ${VAR_NAME}.
func expandEnvVars(input string) string {
	return envVarRegex.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := match[2 : len(match)-1]

		// Check for default value syntax: ${VAR:-default}
		if idx := strings.Index(varName, ":-"); idx != -1 {
			name := varName[:idx]
			defaultVal := varName[idx+2:]
			if val, ok := os.LookupEnv(name); ok {
				return val
			}
			return defaultVal
		}

		// Check for required variable syntax: ${VAR:?error message}
		if idx := strings.Index(varName, ":?"); idx != -1 {
			name := varName[:idx]
			if val, ok := os.LookupEnv(name); ok {
				return val
			}
			// Return empty for unset required vars - validation will catch it
			return ""
		}

		// Simple variable expansion
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}

		// Return original if not found
		return match
	})
}

// MustLoad is like Load but panics on error.
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}
