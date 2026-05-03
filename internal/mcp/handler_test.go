package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
)

func TestFormatGRPCError(t *testing.T) {
	tests := []struct {
		code     codes.Code
		message  string
		expected string
	}{
		{codes.NotFound, "user not found", "[NOT_FOUND] user not found"},
		{codes.InvalidArgument, "invalid input", "[INVALID_ARGUMENT] invalid input"},
		{codes.PermissionDenied, "access denied", "[PERMISSION_DENIED] access denied"},
		{codes.Internal, "internal error", "[INTERNAL] internal error"},
		{codes.Unavailable, "service unavailable", "[UNAVAILABLE] service unavailable"},
		{codes.Unauthenticated, "not authenticated", "[UNAUTHENTICATED] not authenticated"},
		{codes.DeadlineExceeded, "timeout", "[DEADLINE_EXCEEDED] timeout"},
		{codes.ResourceExhausted, "rate limited", "[RESOURCE_EXHAUSTED] rate limited"},
		{codes.FailedPrecondition, "precondition failed", "[FAILED_PRECONDITION] precondition failed"},
		{codes.Aborted, "operation aborted", "[ABORTED] operation aborted"},
		{codes.OutOfRange, "out of range", "[OUT_OF_RANGE] out of range"},
		{codes.Unimplemented, "not implemented", "[UNIMPLEMENTED] not implemented"},
		{codes.DataLoss, "data loss", "[DATA_LOSS] data loss"},
		{codes.AlreadyExists, "already exists", "[ALREADY_EXISTS] already exists"},
		{codes.Canceled, "canceled", "[CANCELED] canceled"},
		{codes.Unknown, "unknown error", "[UNKNOWN] unknown error"},
		{codes.OK, "success", "[OK] success"},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			st := status.New(tt.code, tt.message)
			result := formatGRPCError(st)
			if result != tt.expected {
				t.Errorf("formatGRPCError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	// Test gRPC error
	grpcErr := status.Error(codes.NotFound, "item not found")
	result := formatError(grpcErr)

	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	// Check content contains error message
	if len(result.Content) == 0 {
		t.Error("expected content")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Error("expected text content")
	}

	if textContent.Text != "[NOT_FOUND] item not found" {
		t.Errorf("unexpected error text: %s", textContent.Text)
	}

	// Test non-gRPC error
	regularErr := errTest("regular error")
	result = formatError(regularErr)

	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	textContent, ok = result.Content[0].(mcp.TextContent)
	if !ok {
		t.Error("expected text content")
	}

	if textContent.Text != "regular error" {
		t.Errorf("unexpected error text: %s", textContent.Text)
	}
}

// errTest is a simple error for testing
type errTest string

func (e errTest) Error() string {
	return string(e)
}

func TestFormatSuccess(t *testing.T) {
	tests := []struct {
		name     string
		response interface{}
		validate func(*testing.T, *mcp.CallToolResult)
	}{
		{
			name:     "simple object",
			response: map[string]string{"name": "test", "value": "123"},
			validate: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Error("expected no error")
				}
				if len(result.Content) == 0 {
					t.Error("expected content")
				}
				textContent, ok := result.Content[0].(mcp.TextContent)
				if !ok {
					t.Error("expected text content")
				}
				// Verify it's valid JSON
				var parsed map[string]string
				if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
					t.Errorf("invalid JSON: %v", err)
				}
				if parsed["name"] != "test" {
					t.Errorf("unexpected name: %s", parsed["name"])
				}
			},
		},
		{
			name:     "array response",
			response: []string{"a", "b", "c"},
			validate: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Error("expected no error")
				}
				textContent, ok := result.Content[0].(mcp.TextContent)
				if !ok {
					t.Error("expected text content")
				}
				var parsed []string
				if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
					t.Errorf("invalid JSON: %v", err)
				}
				if len(parsed) != 3 {
					t.Errorf("expected 3 items, got %d", len(parsed))
				}
			},
		},
		{
			name:     "nested object",
			response: map[string]interface{}{"user": map[string]string{"id": "123", "name": "test"}},
			validate: func(t *testing.T, result *mcp.CallToolResult) {
				if result.IsError {
					t.Error("expected no error")
				}
				textContent, ok := result.Content[0].(mcp.TextContent)
				if !ok {
					t.Error("expected text content")
				}
				// Should be pretty-printed JSON
				if len(textContent.Text) == 0 {
					t.Error("expected non-empty text")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatSuccess(tt.response)
			if err != nil {
				t.Fatalf("formatSuccess failed: %v", err)
			}
			tt.validate(t, result)
		})
	}
}

func TestGRPCCodeToString(t *testing.T) {
	// Test all known codes
	testCodes := []codes.Code{
		codes.OK,
		codes.Canceled,
		codes.Unknown,
		codes.InvalidArgument,
		codes.DeadlineExceeded,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.FailedPrecondition,
		codes.Aborted,
		codes.OutOfRange,
		codes.Unimplemented,
		codes.Internal,
		codes.Unavailable,
		codes.DataLoss,
		codes.Unauthenticated,
	}

	for _, code := range testCodes {
		result := grpcCodeToString(code)
		if result == "" {
			t.Errorf("empty result for code %v", code)
		}
		// Should not contain "CODE_" prefix for known codes
		if len(result) >= 5 && result[:5] == "CODE_" && code <= codes.Unauthenticated {
			t.Errorf("unexpected CODE_ prefix for known code %v", code)
		}
	}

	// Test unknown code
	unknownCode := codes.Code(999)
	result := grpcCodeToString(unknownCode)
	if result != "CODE_999" {
		t.Errorf("expected CODE_999 for unknown code, got %s", result)
	}
}

func TestHandler_NewHandler(t *testing.T) {
	handler := NewHandler(nil, "test.Service", "GetItem", false, false)

	if handler.serviceName != "test.Service" {
		t.Errorf("unexpected service name: %s", handler.serviceName)
	}
	if handler.methodName != "GetItem" {
		t.Errorf("unexpected method name: %s", handler.methodName)
	}
	if handler.isClientStream {
		t.Error("unexpected client stream flag")
	}
	if handler.isServerStream {
		t.Error("unexpected server stream flag")
	}

	// Test with streaming
	streamHandler := NewHandler(nil, "test.Service", "Stream", true, true)
	if !streamHandler.isClientStream {
		t.Error("expected client stream flag")
	}
	if !streamHandler.isServerStream {
		t.Error("expected server stream flag")
	}
}

func TestHandler_HandleClientStream_InputValidation(t *testing.T) {
	handler := NewHandler(nil, "test.Service", "BatchCreate", true, false)

	// Test with missing requests field
	args := map[string]any{
		"other": "value",
	}
	_, err := handler.handleClientStream(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing requests")
	}
	if err.Error() != "client streaming requires 'requests' array in input" {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with non-array requests
	args = map[string]any{
		"requests": "not-an-array",
	}
	_, err = handler.handleClientStream(context.Background(), args)
	if err == nil {
		t.Error("expected error for non-array requests")
	}
	if err.Error() != "'requests' must be an array" {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with non-object items in array
	args = map[string]any{
		"requests": []interface{}{"string-item"},
	}
	_, err = handler.handleClientStream(context.Background(), args)
	if err == nil {
		t.Error("expected error for non-object items")
	}
	if err.Error() != "each request must be an object" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestFinalize_PropagatesGRPCStatusFromResult verifies that gRPC status
// errors carried in InvokeResult.Error are surfaced as MCP tool errors
// with the code+message preserved.
//
// Regression test for the silent-null bug: the invoker reports gRPC status
// errors via InvokeResult.Error rather than the returned Go error, and
// before the fix Handle() would treat such results as success and return
// `null` to the MCP client with no diagnostic.
func TestFinalize_PropagatesGRPCStatusFromResult(t *testing.T) {
	cases := []struct {
		name      string
		code      codes.Code
		msg       string
		wantCode  string
		wantInMsg string
	}{
		{"invalid_argument", codes.InvalidArgument, "extracted_data must be valid JSON object",
			"INVALID_ARGUMENT", "extracted_data must be valid JSON object"},
		{"not_found", codes.NotFound, "user 42 not found", "NOT_FOUND", "user 42 not found"},
		{"internal", codes.Internal, "intentional failure for testing", "INTERNAL", "intentional failure"},
		{"unauthenticated", codes.Unauthenticated, "missing token", "UNAUTHENTICATED", "missing token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grpcErr := status.Error(tc.code, tc.msg)
			result := &grpcclient.InvokeResult{Error: grpcErr}

			out, err := finalize(result, nil)
			if err != nil {
				t.Fatalf("finalize returned Go error: %v", err)
			}
			if out == nil {
				t.Fatal("nil result")
			}
			if !out.IsError {
				t.Fatal("expected IsError=true for upstream gRPC error, got false (silent-null bug regression)")
			}
			if len(out.Content) == 0 {
				t.Fatal("expected error content")
			}
			text, ok := out.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("expected TextContent, got %T", out.Content[0])
			}
			if !strings.Contains(text.Text, tc.wantCode) {
				t.Errorf("expected text to contain %q, got %q", tc.wantCode, text.Text)
			}
			if !strings.Contains(text.Text, tc.wantInMsg) {
				t.Errorf("expected text to contain %q, got %q", tc.wantInMsg, text.Text)
			}
		})
	}
}

// TestFinalize_GoErrorPath verifies that setup-time Go errors (returned
// alongside the result) still produce MCP tool errors. This is the path
// that was working before the fix.
func TestFinalize_GoErrorPath(t *testing.T) {
	out, err := finalize(nil, fmtError("setup failed"))
	if err != nil {
		t.Fatalf("finalize returned Go error: %v", err)
	}
	if out == nil || !out.IsError {
		t.Fatal("expected IsError=true")
	}
	text := out.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "setup failed") {
		t.Errorf("expected error text to contain 'setup failed', got %q", text)
	}
}

// TestFinalize_SuccessPath verifies that a populated result with neither
// error path triggered marshals to a successful tool result.
func TestFinalize_SuccessPath(t *testing.T) {
	result := &grpcclient.InvokeResult{Response: map[string]any{"hello": "world"}}
	out, err := finalize(result, nil)
	if err != nil {
		t.Fatalf("finalize returned Go error: %v", err)
	}
	if out == nil || out.IsError {
		t.Fatal("expected IsError=false for successful response")
	}
	text := out.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "\"hello\"") || !strings.Contains(text, "\"world\"") {
		t.Errorf("expected response JSON in text, got %q", text)
	}
}

// fmtError returns an error with the given message. Used to keep
// TestFinalize_GoErrorPath dependency-free of fmt.Errorf style choices.
func fmtError(msg string) error { return errTest(msg) }
