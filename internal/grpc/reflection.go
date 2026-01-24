package grpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// ServiceInfo contains information about a discovered gRPC service.
type ServiceInfo struct {
	FullName string       // Fully qualified service name (e.g., "users.v1.UserService")
	Methods  []MethodInfo // Methods in this service
}

// MethodInfo contains information about a gRPC method.
type MethodInfo struct {
	Name             string                  // Method name (e.g., "GetUser")
	FullName         string                  // Fully qualified name (e.g., "users.v1.UserService.GetUser")
	IsClientStream   bool                    // True if client streams requests
	IsServerStream   bool                    // True if server streams responses
	InputDescriptor  *desc.MessageDescriptor // Input message descriptor
	OutputDescriptor *desc.MessageDescriptor // Output message descriptor
}

// ReflectionClient provides service discovery via gRPC reflection.
type ReflectionClient struct {
	conn *grpc.ClientConn
}

// NewReflectionClient creates a new reflection client for the given connection.
func NewReflectionClient(conn *grpc.ClientConn) *ReflectionClient {
	return &ReflectionClient{conn: conn}
}

// ListServices returns all services available via reflection.
func (r *ReflectionClient) ListServices(ctx context.Context) ([]string, error) {
	stub := rpb.NewServerReflectionClient(r.conn)
	client := grpcreflect.NewClientV1Alpha(ctx, stub)
	defer client.Reset()

	services, err := client.ListServices()
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	return services, nil
}

// DescribeService returns detailed information about a service.
func (r *ReflectionClient) DescribeService(ctx context.Context, serviceName string) (*ServiceInfo, error) {
	stub := rpb.NewServerReflectionClient(r.conn)
	client := grpcreflect.NewClientV1Alpha(ctx, stub)
	defer client.Reset()

	// Resolve the service descriptor
	svc, err := client.ResolveService(serviceName)
	if err != nil {
		return nil, fmt.Errorf("resolving service %s: %w", serviceName, err)
	}

	info := &ServiceInfo{
		FullName: serviceName,
		Methods:  make([]MethodInfo, 0, len(svc.GetMethods())),
	}

	// Get method information
	for _, method := range svc.GetMethods() {
		methodInfo := MethodInfo{
			Name:             method.GetName(),
			FullName:         fmt.Sprintf("%s.%s", serviceName, method.GetName()),
			IsClientStream:   method.IsClientStreaming(),
			IsServerStream:   method.IsServerStreaming(),
			InputDescriptor:  method.GetInputType(),
			OutputDescriptor: method.GetOutputType(),
		}
		info.Methods = append(info.Methods, methodInfo)
	}

	return info, nil
}

// DiscoverServices discovers all services matching the include/exclude patterns.
func (r *ReflectionClient) DiscoverServices(ctx context.Context, include, exclude []string) ([]ServiceInfo, error) {
	allServices, err := r.ListServices(ctx)
	if err != nil {
		return nil, err
	}

	var services []ServiceInfo
	for _, svcName := range allServices {
		// Check if service should be excluded
		if shouldExclude(svcName, include, exclude) {
			continue
		}

		info, err := r.DescribeService(ctx, svcName)
		if err != nil {
			// Log but don't fail - some services might not be fully describable
			continue
		}
		services = append(services, *info)
	}

	return services, nil
}

// shouldExclude determines if a service should be excluded based on patterns.
func shouldExclude(serviceName string, include, exclude []string) bool {
	// If include patterns are specified, service must match at least one
	if len(include) > 0 {
		matched := false
		for _, pattern := range include {
			if matchPattern(serviceName, pattern) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}

	// Check exclude patterns
	for _, pattern := range exclude {
		if matchPattern(serviceName, pattern) {
			return true
		}
	}

	return false
}

// matchPattern performs simple glob matching with * wildcards.
func matchPattern(name, pattern string) bool {
	// Handle exact match
	if pattern == name {
		return true
	}

	// Handle prefix wildcard (e.g., "grpc.reflection.*")
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(name, prefix+".")
	}

	// Handle suffix wildcard (e.g., "*.UserService")
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(name, "."+suffix) || name == suffix
	}

	// Handle contains wildcard (e.g., "*User*")
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		middle := pattern[1 : len(pattern)-1]
		return strings.Contains(name, middle)
	}

	return false
}
