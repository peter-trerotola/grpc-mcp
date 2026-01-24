package mcp

import (
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/builder"

	grpcclient "github.com/grpc-mcp/grpc-mcp/internal/grpc"
)

func TestToolGenerator_GenerateTools(t *testing.T) {
	// Build a test message descriptor
	requestMsg := builder.NewMessage("TestRequest").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("name", builder.FieldTypeString()))

	responseMsg := builder.NewMessage("TestResponse").
		AddField(builder.NewField("result", builder.FieldTypeString()))

	file := builder.NewFile("test.proto").
		SetPackageName("test.v1").
		AddMessage(requestMsg).
		AddMessage(responseMsg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build file: %v", err)
	}

	reqDesc := fd.FindMessage("test.v1.TestRequest")
	if reqDesc == nil {
		t.Fatal("request message not found")
	}

	// Create service info
	serviceInfo := &grpcclient.ServiceInfo{
		FullName: "test.v1.TestService",
		Methods: []grpcclient.MethodInfo{
			{
				Name:            "GetItem",
				FullName:        "/test.v1.TestService/GetItem",
				IsClientStream:  false,
				IsServerStream:  false,
				InputDescriptor: reqDesc,
			},
			{
				Name:            "StreamItems",
				FullName:        "/test.v1.TestService/StreamItems",
				IsClientStream:  false,
				IsServerStream:  true,
				InputDescriptor: reqDesc,
			},
			{
				Name:            "BatchCreate",
				FullName:        "/test.v1.TestService/BatchCreate",
				IsClientStream:  true,
				IsServerStream:  false,
				InputDescriptor: reqDesc,
			},
			{
				Name:            "Chat",
				FullName:        "/test.v1.TestService/Chat",
				IsClientStream:  true,
				IsServerStream:  true,
				InputDescriptor: reqDesc,
			},
		},
	}

	generator := NewToolGenerator()
	registrations := generator.GenerateTools("local-api", serviceInfo, nil)

	if len(registrations) != 4 {
		t.Errorf("expected 4 registrations, got %d", len(registrations))
	}

	// Check first tool (unary)
	unaryTool := registrations[0].Tool
	if unaryTool.Name != "local-api.test.v1.TestService.GetItem" {
		t.Errorf("unexpected tool name: %s", unaryTool.Name)
	}
	if unaryTool.Description != "Call test.v1.TestService.GetItem" {
		t.Errorf("unexpected description: %s", unaryTool.Description)
	}

	// Check server streaming tool
	serverStreamTool := registrations[1].Tool
	if serverStreamTool.Name != "local-api.test.v1.TestService.StreamItems" {
		t.Errorf("unexpected tool name: %s", serverStreamTool.Name)
	}
	expectedDesc := "Call test.v1.TestService.StreamItems (server streaming)"
	if serverStreamTool.Description != expectedDesc {
		t.Errorf("unexpected description: %s, want %s", serverStreamTool.Description, expectedDesc)
	}

	// Check client streaming tool
	clientStreamTool := registrations[2].Tool
	if clientStreamTool.Name != "local-api.test.v1.TestService.BatchCreate" {
		t.Errorf("unexpected tool name: %s", clientStreamTool.Name)
	}
	expectedDesc = "Call test.v1.TestService.BatchCreate (client streaming)"
	if clientStreamTool.Description != expectedDesc {
		t.Errorf("unexpected description: %s, want %s", clientStreamTool.Description, expectedDesc)
	}

	// Check client streaming schema has requests array wrapper
	if clientStreamTool.InputSchema.Properties == nil {
		t.Error("expected input schema properties")
	} else if _, ok := clientStreamTool.InputSchema.Properties["requests"]; !ok {
		t.Error("expected 'requests' property for client streaming")
	}

	// Check bidi streaming tool
	bidiTool := registrations[3].Tool
	if bidiTool.Name != "local-api.test.v1.TestService.Chat" {
		t.Errorf("unexpected tool name: %s", bidiTool.Name)
	}
	expectedDesc = "Call test.v1.TestService.Chat (bidirectional streaming)"
	if bidiTool.Description != expectedDesc {
		t.Errorf("unexpected description: %s, want %s", bidiTool.Description, expectedDesc)
	}
}

func TestToolGenerator_InputSchemaConversion(t *testing.T) {
	// Build a complex message
	nestedMsg := builder.NewMessage("Address").
		AddField(builder.NewField("street", builder.FieldTypeString())).
		AddField(builder.NewField("city", builder.FieldTypeString()))

	statusEnum := builder.NewEnum("Status").
		AddValue(builder.NewEnumValue("UNKNOWN").SetNumber(0)).
		AddValue(builder.NewEnumValue("ACTIVE").SetNumber(1))

	requestMsg := builder.NewMessage("UserRequest").
		AddField(builder.NewField("id", builder.FieldTypeInt64())).
		AddField(builder.NewField("name", builder.FieldTypeString())).
		AddField(builder.NewField("tags", builder.FieldTypeString()).SetRepeated()).
		AddField(builder.NewField("address", builder.FieldTypeMessage(nestedMsg))).
		AddField(builder.NewField("status", builder.FieldTypeEnum(statusEnum)))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(nestedMsg).
		AddMessage(requestMsg).
		AddEnum(statusEnum)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build file: %v", err)
	}

	reqDesc := fd.FindMessage("test.UserRequest")
	serviceInfo := &grpcclient.ServiceInfo{
		FullName: "test.UserService",
		Methods: []grpcclient.MethodInfo{
			{
				Name:            "CreateUser",
				FullName:        "/test.UserService/CreateUser",
				InputDescriptor: reqDesc,
			},
		},
	}

	generator := NewToolGenerator()
	registrations := generator.GenerateTools("api", serviceInfo, nil)

	if len(registrations) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(registrations))
	}

	tool := registrations[0].Tool
	schema := tool.InputSchema

	// Check schema type
	if schema.Type != "object" {
		t.Errorf("expected object type, got %s", schema.Type)
	}

	// Check properties exist
	props := schema.Properties
	if props == nil {
		t.Fatal("expected properties")
	}

	// Check id field (int64 → string per proto3 JSON spec)
	if idProp, ok := props["id"].(map[string]any); ok {
		if idProp["type"] != "string" {
			t.Errorf("expected id to be string type (64-bit), got %v", idProp["type"])
		}
	} else {
		t.Error("expected id property")
	}

	// Check name field
	if nameProp, ok := props["name"].(map[string]any); ok {
		if nameProp["type"] != "string" {
			t.Errorf("expected name to be string type, got %v", nameProp["type"])
		}
	} else {
		t.Error("expected name property")
	}

	// Check tags array field
	if tagsProp, ok := props["tags"].(map[string]any); ok {
		if tagsProp["type"] != "array" {
			t.Errorf("expected tags to be array type, got %v", tagsProp["type"])
		}
		if items, ok := tagsProp["items"].(map[string]any); ok {
			if items["type"] != "string" {
				t.Errorf("expected tags items to be string, got %v", items["type"])
			}
		}
	} else {
		t.Error("expected tags property")
	}

	// Check address nested object
	if addrProp, ok := props["address"].(map[string]any); ok {
		if addrProp["type"] != "object" {
			t.Errorf("expected address to be object type, got %v", addrProp["type"])
		}
	} else {
		t.Error("expected address property")
	}

	// Check status enum
	if statusProp, ok := props["status"].(map[string]any); ok {
		if statusProp["type"] != "string" {
			t.Errorf("expected status to be string type, got %v", statusProp["type"])
		}
		if enum, ok := statusProp["enum"].([]string); ok {
			if len(enum) != 2 {
				t.Errorf("expected 2 enum values, got %d", len(enum))
			}
		}
	} else {
		t.Error("expected status property")
	}
}

func TestConvertPropertyToAny(t *testing.T) {
	tests := []struct {
		name     string
		input    *desc.MessageDescriptor
		expected map[string]string // field name -> expected type
	}{
		{
			name:  "nil input",
			input: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input == nil {
				result := convertPropertyToAny(nil)
				if result != nil {
					t.Error("expected nil result for nil input")
				}
			}
		})
	}
}

func TestToolToJSON(t *testing.T) {
	// Build a simple message
	requestMsg := builder.NewMessage("SimpleRequest").
		AddField(builder.NewField("value", builder.FieldTypeString()))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(requestMsg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build file: %v", err)
	}

	reqDesc := fd.FindMessage("test.SimpleRequest")
	serviceInfo := &grpcclient.ServiceInfo{
		FullName: "test.SimpleService",
		Methods: []grpcclient.MethodInfo{
			{
				Name:            "Call",
				FullName:        "/test.SimpleService/Call",
				InputDescriptor: reqDesc,
			},
		},
	}

	generator := NewToolGenerator()
	registrations := generator.GenerateTools("dev", serviceInfo, nil)

	if len(registrations) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(registrations))
	}

	jsonStr, err := ToolToJSON(registrations[0].Tool)
	if err != nil {
		t.Fatalf("ToolToJSON failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("expected non-empty JSON")
	}

	// Verify it contains expected content
	if !contains(jsonStr, "dev.test.SimpleService.Call") {
		t.Error("expected tool name in JSON")
	}
	if !contains(jsonStr, "value") {
		t.Error("expected 'value' field in JSON")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
