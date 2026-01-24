package naming

import (
	"testing"
)

func TestFormatToolName(t *testing.T) {
	tests := []struct {
		endpoint    string
		serviceName string
		methodName  string
		expected    string
	}{
		{
			endpoint:    "local-api",
			serviceName: "users.v1.UserService",
			methodName:  "GetUser",
			expected:    "local-api.users.v1.UserService.GetUser",
		},
		{
			endpoint:    "prod",
			serviceName: "orders.OrderService",
			methodName:  "CreateOrder",
			expected:    "prod.orders.OrderService.CreateOrder",
		},
		{
			endpoint:    "dev",
			serviceName: "SimpleService",
			methodName:  "Call",
			expected:    "dev.SimpleService.Call",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatToolName(tt.endpoint, tt.serviceName, tt.methodName)
			if result != tt.expected {
				t.Errorf("FormatToolName(%q, %q, %q) = %q, want %q",
					tt.endpoint, tt.serviceName, tt.methodName, result, tt.expected)
			}
		})
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		endpoint    string
		method      string
		fullService string
		service     string
		pkg         string
	}{
		{
			name:        "full path",
			input:       "local-api.users.v1.UserService.GetUser",
			endpoint:    "local-api",
			method:      "GetUser",
			fullService: "users.v1.UserService",
			service:     "UserService",
			pkg:         "users.v1",
		},
		{
			name:        "simple service",
			input:       "dev.SimpleService.Call",
			endpoint:    "dev",
			method:      "Call",
			fullService: "SimpleService",
			service:     "SimpleService",
			pkg:         "",
		},
		{
			name:        "deeply nested",
			input:       "api.com.example.services.v2.UserService.GetUser",
			endpoint:    "api",
			method:      "GetUser",
			fullService: "com.example.services.v2.UserService",
			service:     "UserService",
			pkg:         "com.example.services.v2",
		},
		{
			name:    "too short",
			input:   "endpoint.method",
			wantErr: true,
		},
		{
			name:    "single part",
			input:   "single",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseToolName(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Endpoint != tt.endpoint {
				t.Errorf("endpoint = %q, want %q", result.Endpoint, tt.endpoint)
			}
			if result.Method != tt.method {
				t.Errorf("method = %q, want %q", result.Method, tt.method)
			}
			if result.FullService != tt.fullService {
				t.Errorf("fullService = %q, want %q", result.FullService, tt.fullService)
			}
			if result.Service != tt.service {
				t.Errorf("service = %q, want %q", result.Service, tt.service)
			}
			if result.Package != tt.pkg {
				t.Errorf("package = %q, want %q", result.Package, tt.pkg)
			}
		})
	}
}

func TestToolName_String(t *testing.T) {
	tn := &ToolName{
		Endpoint:    "api",
		FullService: "users.v1.UserService",
		Method:      "GetUser",
	}

	expected := "api.users.v1.UserService.GetUser"
	if tn.String() != expected {
		t.Errorf("String() = %q, want %q", tn.String(), expected)
	}
}

func TestToolName_ServiceAndMethod(t *testing.T) {
	tn := &ToolName{
		Service: "UserService",
		Method:  "GetUser",
	}

	expected := "UserService.GetUser"
	if tn.ServiceAndMethod() != expected {
		t.Errorf("ServiceAndMethod() = %q, want %q", tn.ServiceAndMethod(), expected)
	}
}

func TestFormatDescription(t *testing.T) {
	tests := []struct {
		service        string
		method         string
		clientStream   bool
		serverStream   bool
		expectedSuffix string
	}{
		{
			service:        "UserService",
			method:         "GetUser",
			clientStream:   false,
			serverStream:   false,
			expectedSuffix: "",
		},
		{
			service:        "UserService",
			method:         "StreamUsers",
			clientStream:   false,
			serverStream:   true,
			expectedSuffix: "(server streaming)",
		},
		{
			service:        "UserService",
			method:         "BatchCreate",
			clientStream:   true,
			serverStream:   false,
			expectedSuffix: "(client streaming)",
		},
		{
			service:        "ChatService",
			method:         "Chat",
			clientStream:   true,
			serverStream:   true,
			expectedSuffix: "(bidirectional streaming)",
		},
	}

	for _, tt := range tests {
		name := tt.service + "." + tt.method
		t.Run(name, func(t *testing.T) {
			desc := FormatDescription(tt.service, tt.method, tt.clientStream, tt.serverStream)

			// Check it starts with "Call"
			if desc[:4] != "Call" {
				t.Errorf("expected to start with 'Call', got %q", desc)
			}

			// Check suffix if expected
			if tt.expectedSuffix != "" {
				if !containsSuffix(desc, tt.expectedSuffix) {
					t.Errorf("expected description to contain %q, got %q", tt.expectedSuffix, desc)
				}
			}
		})
	}
}

func containsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"camelCase", "camelCase"},
		{"with-dash", "with_dash"},
		{"with.dot", "with_dot"},
		{"123start", "start"},
		{"with spaces", "withspaces"},
		{"Special@Chars!", "SpecialChars"},
		{"_underscore", "_underscore"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		serviceName string
		methodName  string
		expected    string
	}{
		{"users.v1.UserService", "GetUser", "UserService.GetUser"},
		{"SimpleService", "Call", "SimpleService.Call"},
		{"com.example.v1.ComplexService", "Do", "ComplexService.Do"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ShortName(tt.serviceName, tt.methodName)
			if result != tt.expected {
				t.Errorf("ShortName(%q, %q) = %q, want %q",
					tt.serviceName, tt.methodName, result, tt.expected)
			}
		})
	}
}
