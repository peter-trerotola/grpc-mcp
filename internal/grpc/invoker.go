package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
)

// Invoker provides dynamic RPC invocation capabilities.
type Invoker struct {
	conn *grpc.ClientConn
}

// NewInvoker creates a new dynamic RPC invoker.
func NewInvoker(conn *grpc.ClientConn) *Invoker {
	return &Invoker{conn: conn}
}

// InvokeResult contains the result of an RPC invocation.
type InvokeResult struct {
	Response interface{} // Single response or array of responses
	Error    error       // gRPC error if any
	Metadata metadata.MD // Response metadata
}

// Invoke performs a dynamic RPC call based on the method type.
// For streaming methods:
// - Server streaming: returns array of responses
// - Client streaming: accepts array of requests, returns single response
// - Bidirectional: accepts array of requests, returns array of responses
func (i *Invoker) Invoke(ctx context.Context, serviceName, methodName string, input interface{}) (*InvokeResult, error) {
	// Create reflection client to get method descriptor
	stub := rpb.NewServerReflectionClient(i.conn)
	refClient := grpcreflect.NewClientV1Alpha(ctx, stub)
	defer refClient.Reset()

	// Resolve the service
	svcDesc, err := refClient.ResolveService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("resolving service %s: %w", serviceName, err)
	}

	// Find the method
	methodDesc := svcDesc.FindMethodByName(methodName)
	if methodDesc == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	// Create dynamic stub
	dynStub := grpcdynamic.NewStub(i.conn)

	// Determine method type and invoke accordingly
	isClientStream := methodDesc.IsClientStreaming()
	isServerStream := methodDesc.IsServerStreaming()

	switch {
	case !isClientStream && !isServerStream:
		return i.invokeUnary(ctx, dynStub, methodDesc, input)
	case !isClientStream && isServerStream:
		return i.invokeServerStream(ctx, dynStub, methodDesc, input)
	case isClientStream && !isServerStream:
		return i.invokeClientStream(ctx, dynStub, methodDesc, input)
	default:
		return i.invokeBidiStream(ctx, dynStub, methodDesc, input)
	}
}

// invokeUnary performs a unary RPC call.
func (i *Invoker) invokeUnary(ctx context.Context, stub grpcdynamic.Stub, method *desc.MethodDescriptor, input interface{}) (*InvokeResult, error) {
	// Create request message
	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	inputDesc := method.GetInputType()
	req := msgFactory.NewDynamicMessage(inputDesc)

	// Populate request from input
	if err := populateMessage(req, input); err != nil {
		return nil, fmt.Errorf("populating request: %w", err)
	}

	// Invoke the RPC
	resp, err := stub.InvokeRpc(ctx, method, req)
	if err != nil {
		return &InvokeResult{Error: err}, nil
	}

	// Convert response to JSON-friendly format
	jsonResp, err := messageToJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("converting response: %w", err)
	}

	return &InvokeResult{Response: jsonResp}, nil
}

// invokeServerStream performs a server-streaming RPC call.
func (i *Invoker) invokeServerStream(ctx context.Context, stub grpcdynamic.Stub, method *desc.MethodDescriptor, input interface{}) (*InvokeResult, error) {
	// Create request message
	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	inputDesc := method.GetInputType()
	req := msgFactory.NewDynamicMessage(inputDesc)

	if err := populateMessage(req, input); err != nil {
		return nil, fmt.Errorf("populating request: %w", err)
	}

	// Invoke the streaming RPC
	stream, err := stub.InvokeRpcServerStream(ctx, method, req)
	if err != nil {
		return &InvokeResult{Error: err}, nil
	}

	// Collect all responses
	var responses []interface{}
	for {
		resp, err := stream.RecvMsg()
		if err == io.EOF {
			break
		}
		if err != nil {
			return &InvokeResult{
				Response: responses,
				Error:    err,
			}, nil
		}

		jsonResp, err := messageToJSON(resp)
		if err != nil {
			return nil, fmt.Errorf("converting response: %w", err)
		}
		responses = append(responses, jsonResp)
	}

	return &InvokeResult{Response: responses}, nil
}

// invokeClientStream performs a client-streaming RPC call.
func (i *Invoker) invokeClientStream(ctx context.Context, stub grpcdynamic.Stub, method *desc.MethodDescriptor, input interface{}) (*InvokeResult, error) {
	// Input should be an array of requests
	requests, ok := input.([]interface{})
	if !ok {
		// Try to convert from JSON array
		if arr, ok := input.([]map[string]interface{}); ok {
			requests = make([]interface{}, len(arr))
			for i, v := range arr {
				requests[i] = v
			}
		} else {
			return nil, fmt.Errorf("client streaming requires array of requests")
		}
	}

	// Create message factory
	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	inputDesc := method.GetInputType()

	// Start the streaming call
	stream, err := stub.InvokeRpcClientStream(ctx, method)
	if err != nil {
		return &InvokeResult{Error: err}, nil
	}

	// Send all requests
	for _, reqData := range requests {
		req := msgFactory.NewDynamicMessage(inputDesc)
		if populateErr := populateMessage(req, reqData); populateErr != nil {
			return nil, fmt.Errorf("populating request: %w", populateErr)
		}
		if sendErr := stream.SendMsg(req); sendErr != nil {
			return &InvokeResult{Error: sendErr}, nil
		}
	}

	// Close send and receive response
	resp, err := stream.CloseAndReceive()
	if err != nil {
		return &InvokeResult{Error: err}, nil
	}

	jsonResp, err := messageToJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("converting response: %w", err)
	}

	return &InvokeResult{Response: jsonResp}, nil
}

// invokeBidiStream performs a bidirectional streaming RPC call.
func (i *Invoker) invokeBidiStream(ctx context.Context, stub grpcdynamic.Stub, method *desc.MethodDescriptor, input interface{}) (*InvokeResult, error) {
	// Input should be an array of requests
	requests, ok := input.([]interface{})
	if !ok {
		if arr, ok := input.([]map[string]interface{}); ok {
			requests = make([]interface{}, len(arr))
			for i, v := range arr {
				requests[i] = v
			}
		} else {
			return nil, fmt.Errorf("bidirectional streaming requires array of requests")
		}
	}

	// Create message factory
	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	inputDesc := method.GetInputType()

	// Start the bidirectional stream
	stream, err := stub.InvokeRpcBidiStream(ctx, method)
	if err != nil {
		return &InvokeResult{Error: err}, nil
	}

	// Send all requests
	for _, reqData := range requests {
		req := msgFactory.NewDynamicMessage(inputDesc)
		if err := populateMessage(req, reqData); err != nil {
			return nil, fmt.Errorf("populating request: %w", err)
		}
		if err := stream.SendMsg(req); err != nil {
			if err == io.EOF {
				break
			}
			return &InvokeResult{Error: err}, nil
		}
	}

	// Close the send direction
	if err := stream.CloseSend(); err != nil {
		return &InvokeResult{Error: err}, nil
	}

	// Collect all responses
	var responses []interface{}
	for {
		resp, err := stream.RecvMsg()
		if err == io.EOF {
			break
		}
		if err != nil {
			return &InvokeResult{
				Response: responses,
				Error:    err,
			}, nil
		}

		jsonResp, err := messageToJSON(resp)
		if err != nil {
			return nil, fmt.Errorf("converting response: %w", err)
		}
		responses = append(responses, jsonResp)
	}

	return &InvokeResult{Response: responses}, nil
}

// populateMessage populates a dynamic message from a map or JSON input.
func populateMessage(msg *dynamic.Message, input interface{}) error {
	if input == nil {
		return nil
	}

	// Convert input to JSON bytes if needed
	var jsonBytes []byte
	var err error

	switch v := input.(type) {
	case []byte:
		jsonBytes = v
	case string:
		jsonBytes = []byte(v)
	case map[string]interface{}:
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshaling input: %w", err)
		}
	default:
		jsonBytes, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshaling input: %w", err)
		}
	}

	// Unmarshal JSON into the dynamic message
	if err := msg.UnmarshalJSON(jsonBytes); err != nil {
		return fmt.Errorf("unmarshaling to message: %w", err)
	}

	return nil
}

// messageToJSON converts a protobuf message to a JSON-friendly map.
func messageToJSON(msg interface{}) (interface{}, error) {
	switch m := msg.(type) {
	case *dynamic.Message:
		jsonBytes, err := m.MarshalJSON()
		if err != nil {
			return nil, err
		}
		var result map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &result); err != nil {
			return nil, err
		}
		return result, nil
	default:
		// For other message types, try JSON marshaling
		jsonBytes, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		var result interface{}
		if err := json.Unmarshal(jsonBytes, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// FormatError formats a gRPC error for MCP response.
func FormatError(err error) string {
	if err == nil {
		return ""
	}

	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}

	return fmt.Sprintf("[%s] %s", st.Code().String(), st.Message())
}
