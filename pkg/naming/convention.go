// Package naming provides tool naming conventions for grpc-mcp.
package naming

import (
	"fmt"
	"strings"
)

// ToolName represents a parsed tool name.
type ToolName struct {
	Endpoint    string // Endpoint name (e.g., "local-api")
	Package     string // Package name (e.g., "users.v1")
	Service     string // Service name (e.g., "UserService")
	Method      string // Method name (e.g., "GetUser")
	FullService string // Full service name (e.g., "users.v1.UserService")
}

// FormatToolName creates a tool name from components.
// Format: {endpoint}.{package.service}.{method}
// Example: local-api.users.v1.UserService.GetUser
func FormatToolName(endpoint, serviceName, methodName string) string {
	return fmt.Sprintf("%s.%s.%s", endpoint, serviceName, methodName)
}

// ParseToolName parses a tool name into its components.
func ParseToolName(name string) (*ToolName, error) {
	parts := strings.Split(name, ".")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid tool name format: %s", name)
	}

	// First part is the endpoint
	endpoint := parts[0]

	// Last part is the method
	method := parts[len(parts)-1]

	// Everything in between is the service (package.Service)
	serviceParts := parts[1 : len(parts)-1]
	fullService := strings.Join(serviceParts, ".")

	// Extract package and service name
	var pkg, service string
	if len(serviceParts) >= 2 {
		// Package is everything except the last part
		pkg = strings.Join(serviceParts[:len(serviceParts)-1], ".")
		service = serviceParts[len(serviceParts)-1]
	} else if len(serviceParts) == 1 {
		service = serviceParts[0]
	}

	return &ToolName{
		Endpoint:    endpoint,
		Package:     pkg,
		Service:     service,
		Method:      method,
		FullService: fullService,
	}, nil
}

// String returns the full tool name.
func (t *ToolName) String() string {
	return FormatToolName(t.Endpoint, t.FullService, t.Method)
}

// ServiceAndMethod returns "ServiceName.MethodName".
func (t *ToolName) ServiceAndMethod() string {
	return fmt.Sprintf("%s.%s", t.Service, t.Method)
}

// FormatDescription creates a description for a tool.
func FormatDescription(serviceName, methodName string, isClientStream, isServerStream bool) string {
	var streamDesc string
	switch {
	case isClientStream && isServerStream:
		streamDesc = " (bidirectional streaming)"
	case isClientStream:
		streamDesc = " (client streaming)"
	case isServerStream:
		streamDesc = " (server streaming)"
	}

	return fmt.Sprintf("Call %s.%s%s", serviceName, methodName, streamDesc)
}

// SanitizeName converts a name to a valid identifier.
// Replaces invalid characters with underscores.
// Leading digits are stripped until a valid starting character is found.
func SanitizeName(name string) string {
	var result strings.Builder
	foundStart := false
	for _, r := range name {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		isUnderscore := r == '_'
		isDashOrDot := r == '-' || r == '.'

		if !foundStart {
			// Looking for first valid starting character (letter or underscore)
			if isLetter || isUnderscore {
				foundStart = true
				result.WriteRune(r)
			} else if isDashOrDot {
				foundStart = true
				result.WriteRune('_')
			}
			// Skip digits and other invalid chars at start
		} else {
			// After start, include letters, digits, underscores
			if isLetter || isDigit || isUnderscore {
				result.WriteRune(r)
			} else if isDashOrDot {
				result.WriteRune('_')
			}
			// Skip other invalid chars
		}
	}
	return result.String()
}

// ShortName returns a shortened version of the tool name.
// Format: {service}.{method}
func ShortName(serviceName, methodName string) string {
	// Extract just the service name (last part after .)
	parts := strings.Split(serviceName, ".")
	service := parts[len(parts)-1]
	return fmt.Sprintf("%s.%s", service, methodName)
}
