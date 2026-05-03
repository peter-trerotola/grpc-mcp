package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
)

// Handler handles MCP tool invocations by calling gRPC methods.
type Handler struct {
	invoker        *grpcclient.Invoker
	serviceName    string
	methodName     string
	isClientStream bool
	isServerStream bool
}

// NewHandler creates a new handler for a gRPC method.
func NewHandler(invoker *grpcclient.Invoker, serviceName, methodName string, isClientStream, isServerStream bool) *Handler {
	return &Handler{
		invoker:        invoker,
		serviceName:    serviceName,
		methodName:     methodName,
		isClientStream: isClientStream,
		isServerStream: isServerStream,
	}
}

// Handle processes an MCP tool call request.
func (h *Handler) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get the request arguments
	args := request.GetArguments()

	// Handle different streaming modes
	var result *grpcclient.InvokeResult
	var err error

	switch {
	case h.isClientStream && h.isServerStream:
		// Bidirectional streaming
		result, err = h.handleBidiStream(ctx, args)
	case h.isClientStream:
		// Client streaming
		result, err = h.handleClientStream(ctx, args)
	case h.isServerStream:
		// Server streaming
		result, err = h.handleServerStream(ctx, args)
	default:
		// Unary
		result, err = h.handleUnary(ctx, args)
	}

	return finalize(result, err)
}

// finalize converts the outputs of an Invoker call into an MCP tool result.
//
// Two distinct failure paths must both surface as MCP tool errors:
//   - The Go error returned alongside the result, used for setup failures
//     (reflection, message construction, etc.).
//   - InvokeResult.Error, used by the invoker to carry gRPC status errors
//     from the upstream server (InvalidArgument, NotFound, Internal, ...).
//
// Before this was extracted, only the first path was checked, so non-OK
// gRPC statuses fell through to formatSuccess(result.Response) where
// Response is nil — the MCP client received a successful tool call with
// content "null" and no diagnostic. Now both paths produce an MCP tool
// error containing the gRPC code and message.
func finalize(result *grpcclient.InvokeResult, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return formatError(err), nil
	}
	if result != nil && result.Error != nil {
		return formatError(result.Error), nil
	}
	return formatSuccess(result.Response)
}

// handleUnary handles a unary RPC call.
func (h *Handler) handleUnary(ctx context.Context, args map[string]any) (*grpcclient.InvokeResult, error) {
	return h.invoker.Invoke(ctx, h.serviceName, h.methodName, args)
}

// handleServerStream handles a server streaming RPC call.
// Collects all streamed responses into an array.
func (h *Handler) handleServerStream(ctx context.Context, args map[string]any) (*grpcclient.InvokeResult, error) {
	return h.invoker.Invoke(ctx, h.serviceName, h.methodName, args)
}

// handleClientStream handles a client streaming RPC call.
// Expects args to contain a "requests" array.
func (h *Handler) handleClientStream(ctx context.Context, args map[string]any) (*grpcclient.InvokeResult, error) {
	// Extract the requests array
	requests, ok := args["requests"]
	if !ok {
		return nil, fmt.Errorf("client streaming requires 'requests' array in input")
	}

	requestsSlice, ok := requests.([]interface{})
	if !ok {
		return nil, fmt.Errorf("'requests' must be an array")
	}

	// Convert to slice of maps
	input := make([]map[string]any, len(requestsSlice))
	for i, req := range requestsSlice {
		reqMap, ok := req.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("each request must be an object")
		}
		// Convert map[string]interface{} to map[string]any
		converted := make(map[string]any, len(reqMap))
		for k, v := range reqMap {
			converted[k] = v
		}
		input[i] = converted
	}

	return h.invoker.Invoke(ctx, h.serviceName, h.methodName, input)
}

// handleBidiStream handles a bidirectional streaming RPC call.
// Expects args to contain a "requests" array, returns array of responses.
func (h *Handler) handleBidiStream(ctx context.Context, args map[string]any) (*grpcclient.InvokeResult, error) {
	// Same input handling as client streaming
	return h.handleClientStream(ctx, args)
}

// formatError converts an error to an MCP error result.
func formatError(err error) *mcp.CallToolResult {
	// Check if it's a gRPC error
	if st, ok := status.FromError(err); ok {
		return mcp.NewToolResultError(formatGRPCError(st))
	}

	// Generic error
	return mcp.NewToolResultError(err.Error())
}

// formatGRPCError formats a gRPC status as an error string.
func formatGRPCError(st *status.Status) string {
	code := st.Code()
	message := st.Message()

	// Format: [CODE] message
	return fmt.Sprintf("[%s] %s", grpcCodeToString(code), message)
}

// grpcCodeToString converts a gRPC code to a readable string.
func grpcCodeToString(code codes.Code) string {
	switch code {
	case codes.OK:
		return "OK"
	case codes.Canceled:
		return "CANCELED"
	case codes.Unknown:
		return "UNKNOWN"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.AlreadyExists:
		return "ALREADY_EXISTS"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	case codes.ResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case codes.FailedPrecondition:
		return "FAILED_PRECONDITION"
	case codes.Aborted:
		return "ABORTED"
	case codes.OutOfRange:
		return "OUT_OF_RANGE"
	case codes.Unimplemented:
		return "UNIMPLEMENTED"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.DataLoss:
		return "DATA_LOSS"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	default:
		return fmt.Sprintf("CODE_%d", code)
	}
}

// formatSuccess converts a successful response to an MCP result.
func formatSuccess(response interface{}) (*mcp.CallToolResult, error) {
	// Convert response to JSON
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return mcp.NewToolResultText(string(data)), nil
}
