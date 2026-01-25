package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grpc-mcp/grpc-mcp/internal/testutil"
)

// TestInvoker tests use direct descriptors from the test server
// since dynamic services don't register their file descriptors
// in the global proto registry.

func TestInvoker_Unary_Direct(t *testing.T) {
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.NewClient(ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Get the Echo method descriptor directly
	testSvc := ts.Services()[0] // TestService
	echoMethod := testSvc.FindMethodByName("Echo")
	if echoMethod == nil {
		t.Fatal("Echo method not found")
	}

	// Create request using dynamic message
	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	req := msgFactory.NewDynamicMessage(echoMethod.GetInputType())
	req.SetFieldByName("message", "Hello, World!")
	req.SetFieldByName("uppercase", false)

	// Invoke using grpcdynamic
	stub := grpcdynamic.NewStub(conn)
	resp, err := stub.InvokeRpc(ctx, echoMethod, req)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}

	dynResp, ok := resp.(*dynamic.Message)
	if !ok {
		t.Fatal("expected *dynamic.Message response")
	}
	msg, _ := dynResp.TryGetFieldByName("message")
	if msg != "Hello, World!" {
		t.Errorf("expected message 'Hello, World!', got %v", msg)
	}

	// Test with uppercase
	req.SetFieldByName("uppercase", true)
	resp, err = stub.InvokeRpc(ctx, echoMethod, req)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}

	dynResp, ok = resp.(*dynamic.Message)
	if !ok {
		t.Fatal("expected *dynamic.Message response")
	}
	msg, _ = dynResp.TryGetFieldByName("message")
	if msg != "HELLO, WORLD!" {
		t.Errorf("expected message 'HELLO, WORLD!', got %v", msg)
	}
}

func TestInvoker_UnaryError_Direct(t *testing.T) {
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.NewClient(ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	testSvc := ts.Services()[0]
	failMethod := testSvc.FindMethodByName("FailAlways")
	if failMethod == nil {
		t.Fatal("FailAlways method not found")
	}

	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	req := msgFactory.NewDynamicMessage(failMethod.GetInputType())

	stub := grpcdynamic.NewStub(conn)
	_, err = stub.InvokeRpc(ctx, failMethod, req)
	if err == nil {
		t.Fatal("expected error from FailAlways")
	}

	formatted := FormatError(err)
	if formatted != "[Internal] intentional failure for testing" {
		t.Errorf("expected formatted error, got %q", formatted)
	}
}

func TestInvoker_ServerStream_Direct(t *testing.T) {
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.NewClient(ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	testSvc := ts.Services()[0]
	streamMethod := testSvc.FindMethodByName("StreamNumbers")
	if streamMethod == nil {
		t.Fatal("StreamNumbers method not found")
	}

	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	req := msgFactory.NewDynamicMessage(streamMethod.GetInputType())
	req.SetFieldByName("start", int32(0))
	req.SetFieldByName("end", int32(5))

	stub := grpcdynamic.NewStub(conn)
	stream, err := stub.InvokeRpcServerStream(ctx, streamMethod, req)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}

	var responses []int32
	for {
		resp, err := stream.RecvMsg()
		if err != nil {
			break
		}
		dynResp, ok := resp.(*dynamic.Message)
		if !ok {
			t.Fatal("expected *dynamic.Message response")
		}
		val, _ := dynResp.TryGetFieldByName("value")
		intVal, ok := val.(int32)
		if !ok {
			t.Fatal("expected int32 value")
		}
		responses = append(responses, intVal)
	}

	if len(responses) != 5 {
		t.Errorf("expected 5 responses, got %d", len(responses))
	}

	for i, v := range responses {
		if int(v) != i {
			t.Errorf("expected value %d at index %d, got %d", i, i, v)
		}
	}
}

func TestInvoker_ClientStream_Direct(t *testing.T) {
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.NewClient(ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	testSvc := ts.Services()[0]
	sumMethod := testSvc.FindMethodByName("SumNumbers")
	if sumMethod == nil {
		t.Fatal("SumNumbers method not found")
	}

	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	stub := grpcdynamic.NewStub(conn)

	stream, err := stub.InvokeRpcClientStream(ctx, sumMethod)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}

	// Send numbers 1-5
	for i := int32(1); i <= 5; i++ {
		req := msgFactory.NewDynamicMessage(sumMethod.GetInputType())
		req.SetFieldByName("value", i)
		if sendErr := stream.SendMsg(req); sendErr != nil {
			t.Fatalf("failed to send: %v", sendErr)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		t.Fatalf("failed to receive: %v", err)
	}

	dynResp, ok := resp.(*dynamic.Message)
	if !ok {
		t.Fatal("expected *dynamic.Message response")
	}
	total, _ := dynResp.TryGetFieldByName("total")
	count, _ := dynResp.TryGetFieldByName("count")

	totalVal, ok := total.(int64)
	if !ok {
		t.Fatal("expected int64 total")
	}
	if totalVal != 15 {
		t.Errorf("expected total 15, got %v", totalVal)
	}
	countVal, ok := count.(int32)
	if !ok {
		t.Fatal("expected int32 count")
	}
	if countVal != 5 {
		t.Errorf("expected count 5, got %v", countVal)
	}
}

func TestInvoker_BidiStream_Direct(t *testing.T) {
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.NewClient(ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	testSvc := ts.Services()[0]
	chatMethod := testSvc.FindMethodByName("Chat")
	if chatMethod == nil {
		t.Fatal("Chat method not found")
	}

	msgFactory := dynamic.NewMessageFactoryWithDefaults()
	stub := grpcdynamic.NewStub(conn)

	stream, err := stub.InvokeRpcBidiStream(ctx, chatMethod)
	if err != nil {
		t.Fatalf("failed to invoke: %v", err)
	}

	// Send messages
	messages := []string{"Hello", "Hi there"}
	for i, msg := range messages {
		req := msgFactory.NewDynamicMessage(chatMethod.GetInputType())
		req.SetFieldByName("user", "user"+string(rune('1'+i)))
		req.SetFieldByName("text", msg)
		if err := stream.SendMsg(req); err != nil {
			t.Fatalf("failed to send: %v", err)
		}
	}

	// Close send
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("failed to close send: %v", err)
	}

	// Receive responses
	var responses []string
	for {
		resp, err := stream.RecvMsg()
		if err != nil {
			break
		}
		dynResp, ok := resp.(*dynamic.Message)
		if !ok {
			t.Fatal("expected *dynamic.Message response")
		}
		text, _ := dynResp.TryGetFieldByName("text")
		textVal, ok := text.(string)
		if !ok {
			t.Fatal("expected string text")
		}
		responses = append(responses, textVal)
	}

	if len(responses) != 2 {
		t.Errorf("expected 2 responses, got %d", len(responses))
	}

	if len(responses) > 0 && responses[0] != "Echo: Hello" {
		t.Errorf("expected 'Echo: Hello', got %q", responses[0])
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatError(tt.err)
			if result != tt.expected {
				t.Errorf("FormatError(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}
