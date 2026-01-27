package mcp

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
	"github.com/peter-trerotola/grpc-mcp/internal/schema"
	"github.com/peter-trerotola/grpc-mcp/pkg/naming"
)

// ToolGenerator generates MCP tools from gRPC service information.
type ToolGenerator struct {
	converter *schema.Converter
}

// NewToolGenerator creates a new tool generator.
func NewToolGenerator() *ToolGenerator {
	return &ToolGenerator{
		converter: schema.NewConverter(),
	}
}

// GenerateTools generates MCP tools for a gRPC service.
func (g *ToolGenerator) GenerateTools(endpointName string, service *grpcclient.ServiceInfo, invoker *grpcclient.Invoker) []ToolRegistration {
	var registrations []ToolRegistration

	for _, method := range service.Methods {
		tool := g.generateTool(endpointName, service, method)
		handler := NewHandler(invoker, service.FullName, method.Name, method.IsClientStream, method.IsServerStream)

		registrations = append(registrations, ToolRegistration{
			Tool:    tool,
			Handler: handler,
		})
	}

	return registrations
}

// generateTool creates an MCP tool for a gRPC method.
func (g *ToolGenerator) generateTool(endpointName string, service *grpcclient.ServiceInfo, method grpcclient.MethodInfo) mcp.Tool {
	// Generate tool name
	toolName := naming.FormatToolName(endpointName, service.FullName, method.Name)

	// Generate description
	description := naming.FormatDescription(
		service.FullName,
		method.Name,
		method.IsClientStream,
		method.IsServerStream,
	)

	// Convert input schema
	inputSchema := g.generateInputSchema(method)

	return mcp.Tool{
		Name:        toolName,
		Description: description,
		InputSchema: inputSchema,
	}
}

// generateInputSchema converts a method's input type to MCP tool input schema.
func (g *ToolGenerator) generateInputSchema(method grpcclient.MethodInfo) mcp.ToolInputSchema {
	var inputSchema mcp.ToolInputSchema

	if method.InputDescriptor != nil {
		jsonSchema := g.converter.MessageToSchema(method.InputDescriptor)

		// For client streaming, wrap in array schema
		if method.IsClientStream {
			arraySchema := &schema.JSONSchema{
				Type:        "object",
				Description: "Array of requests for client streaming",
				Properties: map[string]*schema.JSONSchema{
					"requests": {
						Type:  "array",
						Items: jsonSchema,
					},
				},
				Required: []string{"requests"},
			}
			inputSchema = convertToMCPSchema(arraySchema)
		} else {
			inputSchema = convertToMCPSchema(jsonSchema)
		}
	} else {
		// Fallback to empty object schema
		inputSchema = mcp.ToolInputSchema{
			Type:       "object",
			Properties: make(map[string]any),
		}
	}

	return inputSchema
}

// convertToMCPSchema converts our JSONSchema to MCP's ToolInputSchema.
func convertToMCPSchema(js *schema.JSONSchema) mcp.ToolInputSchema {
	result := mcp.ToolInputSchema{
		Type:     js.Type,
		Required: js.Required,
	}

	if len(js.Properties) > 0 {
		props := make(map[string]any)
		for name, prop := range js.Properties {
			props[name] = convertPropertyToAny(prop)
		}
		result.Properties = props
	}

	if len(js.Definitions) > 0 {
		defs := make(map[string]any)
		for name, def := range js.Definitions {
			defs[name] = convertPropertyToAny(def)
		}
		result.Defs = defs
	}

	return result
}

// convertPropertyToAny converts a JSONSchema property to a map for MCP.
func convertPropertyToAny(js *schema.JSONSchema) map[string]any {
	if js == nil {
		return nil
	}

	result := make(map[string]any)

	if js.Type != "" {
		result["type"] = js.Type
	}
	if js.Description != "" {
		result["description"] = js.Description
	}
	if js.Title != "" {
		result["title"] = js.Title
	}
	if js.Format != "" {
		result["format"] = js.Format
	}
	if js.Ref != "" {
		result["$ref"] = js.Ref
	}
	if len(js.Enum) > 0 {
		result["enum"] = js.Enum
	}
	if len(js.Required) > 0 {
		result["required"] = js.Required
	}
	if js.Minimum != nil {
		result["minimum"] = *js.Minimum
	}
	if js.Maximum != nil {
		result["maximum"] = *js.Maximum
	}
	if js.MinLength != nil {
		result["minLength"] = *js.MinLength
	}
	if js.MaxLength != nil {
		result["maxLength"] = *js.MaxLength
	}
	if js.Pattern != "" {
		result["pattern"] = js.Pattern
	}

	if len(js.Properties) > 0 {
		props := make(map[string]any)
		for name, prop := range js.Properties {
			props[name] = convertPropertyToAny(prop)
		}
		result["properties"] = props
	}

	if js.Items != nil {
		result["items"] = convertPropertyToAny(js.Items)
	}

	if js.AdditionalProperties != nil {
		result["additionalProperties"] = convertPropertyToAny(js.AdditionalProperties)
	}

	if len(js.OneOf) > 0 {
		oneOf := make([]any, len(js.OneOf))
		for i, o := range js.OneOf {
			oneOf[i] = convertPropertyToAny(&o)
		}
		result["oneOf"] = oneOf
	}

	if len(js.AnyOf) > 0 {
		anyOf := make([]any, len(js.AnyOf))
		for i, a := range js.AnyOf {
			anyOf[i] = convertPropertyToAny(&a)
		}
		result["anyOf"] = anyOf
	}

	return result
}

// ToolToJSON converts a tool to JSON for debugging.
func ToolToJSON(tool mcp.Tool) (string, error) {
	data, err := json.MarshalIndent(tool, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
