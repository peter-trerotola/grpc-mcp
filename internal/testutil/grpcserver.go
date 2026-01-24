// Package testutil provides testing utilities including a mock gRPC server.
package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/builder"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// TestServer is a gRPC server for testing with reflection enabled.
type TestServer struct {
	server   *grpc.Server
	listener net.Listener
	addr     string
	files    []*desc.FileDescriptor
	services []*desc.ServiceDescriptor
}

// NewTestServer creates a new test gRPC server on a random available port.
func NewTestServer() (*TestServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	server := grpc.NewServer()

	ts := &TestServer{
		server:   server,
		listener: listener,
		addr:     listener.Addr().String(),
	}

	// Build and register test services
	if err := ts.registerTestServices(); err != nil {
		listener.Close()
		return nil, err
	}

	// Enable reflection - services must be registered first
	reflection.Register(server)

	return ts, nil
}

// Address returns the server's address.
func (ts *TestServer) Address() string {
	return ts.addr
}

// Start starts the server in a goroutine.
func (ts *TestServer) Start() {
	go func() {
		_ = ts.server.Serve(ts.listener)
	}()
}

// Stop gracefully stops the server.
func (ts *TestServer) Stop() {
	ts.server.GracefulStop()
}

// Services returns the service descriptors for direct access.
func (ts *TestServer) Services() []*desc.ServiceDescriptor {
	return ts.services
}

// registerTestServices builds and registers test services.
func (ts *TestServer) registerTestServices() error {
	// Build TestService
	testFile, testSvc, err := ts.buildTestService()
	if err != nil {
		return err
	}
	ts.files = append(ts.files, testFile)
	ts.services = append(ts.services, testSvc)

	// Build ComplexService
	complexFile, complexSvc, err := ts.buildComplexService()
	if err != nil {
		return err
	}
	ts.files = append(ts.files, complexFile)
	ts.services = append(ts.services, complexSvc)

	// Register gRPC services
	for _, svc := range ts.services {
		ts.registerService(svc)
	}

	return nil
}

// buildTestService builds the TestService descriptor.
func (ts *TestServer) buildTestService() (*desc.FileDescriptor, *desc.ServiceDescriptor, error) {
	// Build message types
	echoReq := builder.NewMessage("EchoRequest").
		AddField(builder.NewField("message", builder.FieldTypeString())).
		AddField(builder.NewField("count", builder.FieldTypeInt32())).
		AddField(builder.NewField("uppercase", builder.FieldTypeBool()))

	echoResp := builder.NewMessage("EchoResponse").
		AddField(builder.NewField("message", builder.FieldTypeString())).
		AddField(builder.NewField("length", builder.FieldTypeInt32()))

	streamReq := builder.NewMessage("StreamRequest").
		AddField(builder.NewField("start", builder.FieldTypeInt32())).
		AddField(builder.NewField("end", builder.FieldTypeInt32()))

	numberResp := builder.NewMessage("NumberResponse").
		AddField(builder.NewField("value", builder.FieldTypeInt32()))

	numberReq := builder.NewMessage("NumberRequest").
		AddField(builder.NewField("value", builder.FieldTypeInt32()))

	sumResp := builder.NewMessage("SumResponse").
		AddField(builder.NewField("total", builder.FieldTypeInt64())).
		AddField(builder.NewField("count", builder.FieldTypeInt32()))

	chatMsg := builder.NewMessage("ChatMessage").
		AddField(builder.NewField("user", builder.FieldTypeString())).
		AddField(builder.NewField("text", builder.FieldTypeString())).
		AddField(builder.NewField("timestamp", builder.FieldTypeInt64()))

	// Build service
	svc := builder.NewService("TestService").
		AddMethod(builder.NewMethod("Echo",
			builder.RpcTypeMessage(echoReq, false),
			builder.RpcTypeMessage(echoResp, false))).
		AddMethod(builder.NewMethod("StreamNumbers",
			builder.RpcTypeMessage(streamReq, false),
			builder.RpcTypeMessage(numberResp, true))).
		AddMethod(builder.NewMethod("SumNumbers",
			builder.RpcTypeMessage(numberReq, true),
			builder.RpcTypeMessage(sumResp, false))).
		AddMethod(builder.NewMethod("Chat",
			builder.RpcTypeMessage(chatMsg, true),
			builder.RpcTypeMessage(chatMsg, true))).
		AddMethod(builder.NewMethod("FailAlways",
			builder.RpcTypeMessage(echoReq, false),
			builder.RpcTypeMessage(echoResp, false)))

	// Build file descriptor
	file := builder.NewFile("testservice.proto").
		SetPackageName("testutil").
		AddMessage(echoReq).
		AddMessage(echoResp).
		AddMessage(streamReq).
		AddMessage(numberResp).
		AddMessage(numberReq).
		AddMessage(sumResp).
		AddMessage(chatMsg).
		AddService(svc)

	fd, err := file.Build()
	if err != nil {
		return nil, nil, fmt.Errorf("building test service: %w", err)
	}

	return fd, fd.FindService("testutil.TestService"), nil
}

// buildComplexService builds the ComplexService descriptor.
func (ts *TestServer) buildComplexService() (*desc.FileDescriptor, *desc.ServiceDescriptor, error) {
	// Build nested message
	nestedMsg := builder.NewMessage("NestedMessage").
		AddField(builder.NewField("field1", builder.FieldTypeString())).
		AddField(builder.NewField("field2", builder.FieldTypeInt32()))

	// Build enum
	statusEnum := builder.NewEnum("Status").
		AddValue(builder.NewEnumValue("STATUS_UNKNOWN").SetNumber(0)).
		AddValue(builder.NewEnumValue("STATUS_ACTIVE").SetNumber(1)).
		AddValue(builder.NewEnumValue("STATUS_INACTIVE").SetNumber(2))

	// Build complex request
	complexReq := builder.NewMessage("ComplexRequest").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("nested", builder.FieldTypeMessage(nestedMsg))).
		AddField(builder.NewField("tags", builder.FieldTypeString()).SetRepeated()).
		AddField(builder.NewField("status", builder.FieldTypeEnum(statusEnum)))

	// Build complex response
	complexResp := builder.NewMessage("ComplexResponse").
		AddField(builder.NewField("result", builder.FieldTypeString())).
		AddField(builder.NewField("items", builder.FieldTypeMessage(nestedMsg)).SetRepeated())

	// Build service
	svc := builder.NewService("ComplexService").
		AddMethod(builder.NewMethod("Process",
			builder.RpcTypeMessage(complexReq, false),
			builder.RpcTypeMessage(complexResp, false)))

	// Build file descriptor
	file := builder.NewFile("complexservice.proto").
		SetPackageName("testutil").
		AddMessage(nestedMsg).
		AddMessage(complexReq).
		AddMessage(complexResp).
		AddEnum(statusEnum).
		AddService(svc)

	fd, err := file.Build()
	if err != nil {
		return nil, nil, fmt.Errorf("building complex service: %w", err)
	}

	return fd, fd.FindService("testutil.ComplexService"), nil
}

// registerService registers a service with handlers.
func (ts *TestServer) registerService(svc *desc.ServiceDescriptor) {
	sd := &grpc.ServiceDesc{
		ServiceName: svc.GetFullyQualifiedName(),
		HandlerType: (*interface{})(nil),
		Methods:     make([]grpc.MethodDesc, 0),
		Streams:     make([]grpc.StreamDesc, 0),
	}

	for _, method := range svc.GetMethods() {
		if method.IsClientStreaming() || method.IsServerStreaming() {
			sd.Streams = append(sd.Streams, grpc.StreamDesc{
				StreamName:    method.GetName(),
				Handler:       ts.createStreamHandler(method),
				ServerStreams: method.IsServerStreaming(),
				ClientStreams: method.IsClientStreaming(),
			})
		} else {
			sd.Methods = append(sd.Methods, grpc.MethodDesc{
				MethodName: method.GetName(),
				Handler:    ts.createUnaryHandler(method),
			})
		}
	}

	ts.server.RegisterService(sd, nil)
}

// createUnaryHandler creates a unary handler for a method.
func (ts *TestServer) createUnaryHandler(method *desc.MethodDescriptor) func(interface{}, context.Context, func(interface{}) error, grpc.UnaryServerInterceptor) (interface{}, error) {
	return func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		req := dynamic.NewMessage(method.GetInputType())
		if err := dec(req); err != nil {
			return nil, err
		}

		resp := dynamic.NewMessage(method.GetOutputType())
		methodName := method.GetName()

		switch methodName {
		case "Echo":
			msg, _ := req.TryGetFieldByName("message")
			msgStr, _ := msg.(string)
			uppercase, _ := req.TryGetFieldByName("uppercase")
			if uppercase == true {
				msgStr = strings.ToUpper(msgStr)
			}
			resp.SetFieldByName("message", msgStr)
			resp.SetFieldByName("length", int32(len(msgStr)))

		case "FailAlways":
			return nil, status.Error(codes.Internal, "intentional failure for testing")

		case "Process":
			id, _ := req.TryGetFieldByName("id")
			resp.SetFieldByName("result", fmt.Sprintf("Processed: %v", id))

		default:
			// Generic response
		}

		return resp, nil
	}
}

// createStreamHandler creates a stream handler for a method.
func (ts *TestServer) createStreamHandler(method *desc.MethodDescriptor) func(interface{}, grpc.ServerStream) error {
	return func(srv interface{}, stream grpc.ServerStream) error {
		if method.IsClientStreaming() && !method.IsServerStreaming() {
			// Client streaming (SumNumbers)
			return ts.handleClientStream(stream, method)
		} else if !method.IsClientStreaming() && method.IsServerStreaming() {
			// Server streaming (StreamNumbers)
			return ts.handleServerStream(stream, method)
		} else {
			// Bidirectional streaming (Chat)
			return ts.handleBidiStream(stream, method)
		}
	}
}

// handleClientStream handles client streaming RPCs.
func (ts *TestServer) handleClientStream(stream grpc.ServerStream, method *desc.MethodDescriptor) error {
	var total int64
	var count int32

	for {
		req := dynamic.NewMessage(method.GetInputType())
		if err := stream.RecvMsg(req); err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		value, _ := req.TryGetFieldByName("value")
		if v, ok := value.(int32); ok {
			total += int64(v)
			count++
		}
	}

	resp := dynamic.NewMessage(method.GetOutputType())
	resp.SetFieldByName("total", total)
	resp.SetFieldByName("count", count)

	return stream.SendMsg(resp)
}

// handleServerStream handles server streaming RPCs.
func (ts *TestServer) handleServerStream(stream grpc.ServerStream, method *desc.MethodDescriptor) error {
	req := dynamic.NewMessage(method.GetInputType())
	if err := stream.RecvMsg(req); err != nil {
		return err
	}

	start := int32(0)
	end := int32(5)
	if v, err := req.TryGetFieldByName("start"); err == nil {
		if s, ok := v.(int32); ok {
			start = s
		}
	}
	if v, err := req.TryGetFieldByName("end"); err == nil {
		if e, ok := v.(int32); ok {
			end = e
		}
	}

	for i := start; i < end; i++ {
		resp := dynamic.NewMessage(method.GetOutputType())
		resp.SetFieldByName("value", i)
		if err := stream.SendMsg(resp); err != nil {
			return err
		}
	}

	return nil
}

// handleBidiStream handles bidirectional streaming RPCs.
func (ts *TestServer) handleBidiStream(stream grpc.ServerStream, method *desc.MethodDescriptor) error {
	for {
		req := dynamic.NewMessage(method.GetInputType())
		if err := stream.RecvMsg(req); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		resp := dynamic.NewMessage(method.GetOutputType())

		// Echo back with modifications
		if user, err := req.TryGetFieldByName("user"); err == nil {
			resp.SetFieldByName("user", user)
		}
		if text, err := req.TryGetFieldByName("text"); err == nil {
			if t, ok := text.(string); ok {
				resp.SetFieldByName("text", "Echo: "+t)
			}
		}
		resp.SetFieldByName("timestamp", time.Now().Unix())

		if err := stream.SendMsg(resp); err != nil {
			return err
		}
	}
}
