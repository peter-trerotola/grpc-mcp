package schema

import (
	"fmt"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Converter converts protobuf descriptors to JSON Schema.
type Converter struct {
	// visited tracks visited message types to handle recursion
	visited map[string]bool
	// definitions stores reusable schema definitions
	definitions map[string]*JSONSchema
}

// NewConverter creates a new schema converter.
func NewConverter() *Converter {
	return &Converter{
		visited:     make(map[string]bool),
		definitions: make(map[string]*JSONSchema),
	}
}

// MessageToSchema converts a message descriptor to a JSON Schema.
func (c *Converter) MessageToSchema(msg *desc.MessageDescriptor) *JSONSchema {
	c.visited = make(map[string]bool)
	c.definitions = make(map[string]*JSONSchema)

	schema := c.convertMessage(msg)

	// Add definitions if any were created
	if len(c.definitions) > 0 {
		schema.Definitions = c.definitions
	}

	return schema
}

// MethodInputSchema returns the JSON Schema for a method's input message.
func (c *Converter) MethodInputSchema(method *desc.MethodDescriptor) *JSONSchema {
	inputType := method.GetInputType()
	schema := c.MessageToSchema(inputType)
	schema.Title = inputType.GetName()
	return schema
}

// MethodOutputSchema returns the JSON Schema for a method's output message.
func (c *Converter) MethodOutputSchema(method *desc.MethodDescriptor) *JSONSchema {
	outputType := method.GetOutputType()
	schema := c.MessageToSchema(outputType)
	schema.Title = outputType.GetName()
	return schema
}

// convertMessage converts a message descriptor to a schema.
func (c *Converter) convertMessage(msg *desc.MessageDescriptor) *JSONSchema {
	fullName := msg.GetFullyQualifiedName()

	// Handle recursion - if we've seen this message before, use a reference
	if c.visited[fullName] {
		return &JSONSchema{
			Ref: "#/$defs/" + sanitizeRefName(fullName),
		}
	}
	c.visited[fullName] = true

	schema := NewObjectSchema()
	schema.Title = msg.GetName()

	if comment := msg.GetSourceInfo().GetLeadingComments(); comment != "" {
		schema.Description = strings.TrimSpace(comment)
	}

	// Handle oneofs
	oneofSchemas := c.collectOneofs(msg)

	// Process fields
	for _, field := range msg.GetFields() {
		// Skip fields that are part of a oneof (they're handled separately)
		if field.GetOneOf() != nil && !field.AsFieldDescriptorProto().GetProto3Optional() {
			continue
		}

		fieldSchema := c.convertField(field)
		schema.AddProperty(c.fieldName(field), fieldSchema, false)
	}

	// Add oneof schemas
	if len(oneofSchemas) > 0 {
		for name, oneofSchema := range oneofSchemas {
			schema.Properties[name] = oneofSchema
		}
	}

	// Store in definitions for potential reuse
	c.definitions[sanitizeRefName(fullName)] = schema

	return schema
}

// collectOneofs collects oneof field groups.
func (c *Converter) collectOneofs(msg *desc.MessageDescriptor) map[string]*JSONSchema {
	oneofs := make(map[string]*JSONSchema)

	for _, oneof := range msg.GetOneOfs() {
		// Skip synthetic oneofs (proto3 optional)
		if oneof.IsSynthetic() {
			continue
		}

		oneofSchema := &JSONSchema{
			Description: fmt.Sprintf("One of: %s", oneof.GetName()),
			OneOf:       make([]JSONSchema, 0),
		}

		for _, field := range oneof.GetChoices() {
			fieldSchema := c.convertField(field)
			wrapper := NewObjectSchema()
			wrapper.AddProperty(c.fieldName(field), fieldSchema, false)
			oneofSchema.OneOf = append(oneofSchema.OneOf, *wrapper)
		}

		oneofs[oneof.GetName()] = oneofSchema
	}

	return oneofs
}

// convertField converts a field descriptor to a schema.
func (c *Converter) convertField(field *desc.FieldDescriptor) *JSONSchema {
	var schema *JSONSchema

	// Handle map fields
	if field.IsMap() {
		schema = c.convertMapField(field)
	} else {
		// Convert the base type
		schema = c.convertFieldType(field)

		// Handle repeated fields (arrays)
		if field.IsRepeated() && !field.IsMap() {
			schema = NewArraySchema(schema)
		}
	}

	// Add description from comments
	if comment := field.GetSourceInfo().GetLeadingComments(); comment != "" {
		schema.Description = strings.TrimSpace(comment)
	}

	return schema
}

// convertFieldType converts a field type to a schema.
func (c *Converter) convertFieldType(field *desc.FieldDescriptor) *JSONSchema {
	fieldType := field.GetType()

	switch fieldType {
	// Floating-point types
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return NewNumberSchema()

	// Integer types that fit in JSON number
	case descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return NewIntegerSchema()

	// Large integer types - use string per proto3 JSON spec
	case descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return &JSONSchema{
			Type:        "string",
			Description: "64-bit integer encoded as string",
		}

	// Boolean
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return NewBooleanSchema()

	// String
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return NewStringSchema()

	// Bytes - base64 encoded
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return &JSONSchema{
			Type:   "string",
			Format: "byte",
		}

	// Enum
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return c.convertEnum(field.GetEnumType())

	// Message
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
		descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return c.convertMessage(field.GetMessageType())

	default:
		return NewStringSchema()
	}
}

// convertMapField converts a map field to a schema.
func (c *Converter) convertMapField(field *desc.FieldDescriptor) *JSONSchema {
	mapEntry := field.GetMessageType()

	// Get value field type (field number 2 in map entry)
	valueField := mapEntry.FindFieldByNumber(2)
	valueSchema := c.convertFieldType(valueField)

	return &JSONSchema{
		Type:                 "object",
		AdditionalProperties: valueSchema,
	}
}

// convertEnum converts an enum descriptor to a schema.
func (c *Converter) convertEnum(enum *desc.EnumDescriptor) *JSONSchema {
	values := make([]string, 0, len(enum.GetValues()))
	for _, v := range enum.GetValues() {
		values = append(values, v.GetName())
	}
	return NewEnumSchema(values)
}

// fieldName returns the JSON field name for a proto field.
func (c *Converter) fieldName(field *desc.FieldDescriptor) string {
	// Use the JSON name if available, otherwise use the proto name
	jsonName := field.GetJSONName()
	if jsonName != "" {
		return jsonName
	}
	return field.GetName()
}

// sanitizeRefName creates a valid JSON Schema $ref name.
func sanitizeRefName(name string) string {
	return strings.ReplaceAll(name, ".", "_")
}

// FieldTypeToJSONType returns the JSON Schema type for a protobuf field type.
func FieldTypeToJSONType(fieldType descriptorpb.FieldDescriptorProto_Type) string {
	switch fieldType {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "number"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "integer"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "string" // 64-bit as string per proto3 JSON spec
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "boolean"
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE,
		descriptorpb.FieldDescriptorProto_TYPE_GROUP:
		return "object"
	default:
		return "string"
	}
}
