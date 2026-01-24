// Package schema provides JSON Schema types and proto-to-JSON-Schema conversion.
package schema

// JSONSchema represents a JSON Schema definition.
type JSONSchema struct {
	// Core schema properties
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Title       string      `json:"title,omitempty"`
	Default     interface{} `json:"default,omitempty"`

	// Object properties
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *JSONSchema            `json:"additionalProperties,omitempty"`

	// Array properties
	Items *JSONSchema `json:"items,omitempty"`

	// String properties
	Enum      []string `json:"enum,omitempty"`
	Format    string   `json:"format,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`

	// Numeric properties
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`

	// Schema composition
	OneOf []JSONSchema `json:"oneOf,omitempty"`
	AnyOf []JSONSchema `json:"anyOf,omitempty"`

	// Reference
	Ref string `json:"$ref,omitempty"`

	// Definitions for reusable schemas
	Definitions map[string]*JSONSchema `json:"$defs,omitempty"`
}

// NewObjectSchema creates a new object schema.
func NewObjectSchema() *JSONSchema {
	return &JSONSchema{
		Type:       "object",
		Properties: make(map[string]*JSONSchema),
	}
}

// NewArraySchema creates a new array schema with the given items schema.
func NewArraySchema(items *JSONSchema) *JSONSchema {
	return &JSONSchema{
		Type:  "array",
		Items: items,
	}
}

// NewStringSchema creates a new string schema.
func NewStringSchema() *JSONSchema {
	return &JSONSchema{Type: "string"}
}

// NewIntegerSchema creates a new integer schema.
func NewIntegerSchema() *JSONSchema {
	return &JSONSchema{Type: "integer"}
}

// NewNumberSchema creates a new number schema.
func NewNumberSchema() *JSONSchema {
	return &JSONSchema{Type: "number"}
}

// NewBooleanSchema creates a new boolean schema.
func NewBooleanSchema() *JSONSchema {
	return &JSONSchema{Type: "boolean"}
}

// NewEnumSchema creates a new string schema with enum values.
func NewEnumSchema(values []string) *JSONSchema {
	return &JSONSchema{
		Type: "string",
		Enum: values,
	}
}

// AddProperty adds a property to an object schema.
func (s *JSONSchema) AddProperty(name string, prop *JSONSchema, required bool) {
	if s.Properties == nil {
		s.Properties = make(map[string]*JSONSchema)
	}
	s.Properties[name] = prop
	if required {
		s.Required = append(s.Required, name)
	}
}

// WithDescription sets the description and returns the schema.
func (s *JSONSchema) WithDescription(desc string) *JSONSchema {
	s.Description = desc
	return s
}

// WithTitle sets the title and returns the schema.
func (s *JSONSchema) WithTitle(title string) *JSONSchema {
	s.Title = title
	return s
}

// WithFormat sets the format and returns the schema.
func (s *JSONSchema) WithFormat(format string) *JSONSchema {
	s.Format = format
	return s
}
