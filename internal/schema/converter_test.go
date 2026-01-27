package schema

import (
	"sync"
	"testing"

	"github.com/jhump/protoreflect/desc/builder"
)

func TestConverter_ScalarTypes(t *testing.T) {
	// Build a message with all scalar types
	msg := builder.NewMessage("ScalarMessage").
		AddField(builder.NewField("string_field", builder.FieldTypeString())).
		AddField(builder.NewField("int32_field", builder.FieldTypeInt32())).
		AddField(builder.NewField("int64_field", builder.FieldTypeInt64())).
		AddField(builder.NewField("uint32_field", builder.FieldTypeUInt32())).
		AddField(builder.NewField("uint64_field", builder.FieldTypeUInt64())).
		AddField(builder.NewField("float_field", builder.FieldTypeFloat())).
		AddField(builder.NewField("double_field", builder.FieldTypeDouble())).
		AddField(builder.NewField("bool_field", builder.FieldTypeBool())).
		AddField(builder.NewField("bytes_field", builder.FieldTypeBytes()))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(msg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.ScalarMessage")
	if msgDesc == nil {
		t.Fatal("message not found")
	}

	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	// Check schema type
	if schema.Type != "object" {
		t.Errorf("expected object type, got %s", schema.Type)
	}

	// Check properties
	tests := map[string]string{
		"stringField": "string",
		"int32Field":  "integer",
		"int64Field":  "string", // 64-bit as string
		"uint32Field": "integer",
		"uint64Field": "string", // 64-bit as string
		"floatField":  "number",
		"doubleField": "number",
		"boolField":   "boolean",
		"bytesField":  "string",
	}

	for field, expectedType := range tests {
		prop, ok := schema.Properties[field]
		if !ok {
			t.Errorf("property %s not found", field)
			continue
		}
		if prop.Type != expectedType {
			t.Errorf("field %s: expected type %s, got %s", field, expectedType, prop.Type)
		}
	}

	// Check bytes format
	if schema.Properties["bytesField"].Format != "byte" {
		t.Errorf("expected byte format for bytes field")
	}
}

func TestConverter_RepeatedFields(t *testing.T) {
	msg := builder.NewMessage("RepeatedMessage").
		AddField(builder.NewField("strings", builder.FieldTypeString()).SetRepeated()).
		AddField(builder.NewField("numbers", builder.FieldTypeInt32()).SetRepeated())

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(msg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.RepeatedMessage")
	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	// Check strings array
	stringsProp := schema.Properties["strings"]
	if stringsProp.Type != "array" {
		t.Errorf("expected array type, got %s", stringsProp.Type)
	}
	if stringsProp.Items == nil {
		t.Error("expected items schema")
	} else if stringsProp.Items.Type != "string" {
		t.Errorf("expected string items, got %s", stringsProp.Items.Type)
	}

	// Check numbers array
	numbersProp := schema.Properties["numbers"]
	if numbersProp.Type != "array" {
		t.Errorf("expected array type, got %s", numbersProp.Type)
	}
	if numbersProp.Items == nil {
		t.Error("expected items schema")
	} else if numbersProp.Items.Type != "integer" {
		t.Errorf("expected integer items, got %s", numbersProp.Items.Type)
	}
}

func TestConverter_NestedMessage(t *testing.T) {
	inner := builder.NewMessage("Inner").
		AddField(builder.NewField("value", builder.FieldTypeString()))

	outer := builder.NewMessage("Outer").
		AddField(builder.NewField("inner", builder.FieldTypeMessage(inner))).
		AddField(builder.NewField("name", builder.FieldTypeString()))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(inner).
		AddMessage(outer)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.Outer")
	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	// Check outer schema
	if schema.Type != "object" {
		t.Errorf("expected object type, got %s", schema.Type)
	}

	// Check inner field is object
	innerProp := schema.Properties["inner"]
	if innerProp == nil {
		t.Fatal("inner property not found")
	}
	if innerProp.Type != "object" {
		t.Errorf("expected object type for inner, got %s", innerProp.Type)
	}

	// Check inner has value field
	if innerProp.Properties["value"] == nil {
		t.Error("expected value property in inner")
	}
}

func TestConverter_Enum(t *testing.T) {
	statusEnum := builder.NewEnum("Status").
		AddValue(builder.NewEnumValue("UNKNOWN").SetNumber(0)).
		AddValue(builder.NewEnumValue("ACTIVE").SetNumber(1)).
		AddValue(builder.NewEnumValue("INACTIVE").SetNumber(2))

	msg := builder.NewMessage("EnumMessage").
		AddField(builder.NewField("status", builder.FieldTypeEnum(statusEnum)))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddEnum(statusEnum).
		AddMessage(msg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.EnumMessage")
	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	statusProp := schema.Properties["status"]
	if statusProp == nil {
		t.Fatal("status property not found")
	}
	if statusProp.Type != "string" {
		t.Errorf("expected string type for enum, got %s", statusProp.Type)
	}
	if len(statusProp.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(statusProp.Enum))
	}

	// Check enum values
	expected := []string{"UNKNOWN", "ACTIVE", "INACTIVE"}
	for i, v := range expected {
		if i >= len(statusProp.Enum) || statusProp.Enum[i] != v {
			t.Errorf("expected enum value %s at index %d", v, i)
		}
	}
}

func TestConverter_Map(t *testing.T) {
	msg := builder.NewMessage("MapMessage").
		AddField(builder.NewMapField("metadata", builder.FieldTypeString(), builder.FieldTypeString())).
		AddField(builder.NewMapField("counts", builder.FieldTypeString(), builder.FieldTypeInt32()))

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(msg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.MapMessage")
	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	// Check string-string map
	metadataProp := schema.Properties["metadata"]
	if metadataProp == nil {
		t.Fatal("metadata property not found")
	}
	if metadataProp.Type != "object" {
		t.Errorf("expected object type for map, got %s", metadataProp.Type)
	}
	if metadataProp.AdditionalProperties == nil {
		t.Error("expected additionalProperties for map")
	} else if metadataProp.AdditionalProperties.Type != "string" {
		t.Errorf("expected string value type, got %s", metadataProp.AdditionalProperties.Type)
	}

	// Check string-int32 map
	countsProp := schema.Properties["counts"]
	if countsProp == nil {
		t.Fatal("counts property not found")
	}
	if countsProp.AdditionalProperties == nil {
		t.Error("expected additionalProperties for map")
	} else if countsProp.AdditionalProperties.Type != "integer" {
		t.Errorf("expected integer value type, got %s", countsProp.AdditionalProperties.Type)
	}
}

func TestConverter_RepeatedNestedMessage(t *testing.T) {
	item := builder.NewMessage("Item").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("name", builder.FieldTypeString()))

	list := builder.NewMessage("List").
		AddField(builder.NewField("items", builder.FieldTypeMessage(item)).SetRepeated())

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(item).
		AddMessage(list)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.List")
	conv := NewConverter()
	schema := conv.MessageToSchema(msgDesc)

	itemsProp := schema.Properties["items"]
	if itemsProp == nil {
		t.Fatal("items property not found")
	}
	if itemsProp.Type != "array" {
		t.Errorf("expected array type, got %s", itemsProp.Type)
	}
	if itemsProp.Items == nil {
		t.Fatal("expected items schema")
	}
	if itemsProp.Items.Type != "object" {
		t.Errorf("expected object type for items, got %s", itemsProp.Items.Type)
	}
}

func TestJSONSchemaHelpers(t *testing.T) {
	// Test NewObjectSchema
	obj := NewObjectSchema()
	if obj.Type != "object" {
		t.Errorf("expected object type, got %s", obj.Type)
	}
	if obj.Properties == nil {
		t.Error("expected properties to be initialized")
	}

	// Test AddProperty
	obj.AddProperty("name", NewStringSchema(), true)
	if len(obj.Properties) != 1 {
		t.Error("expected 1 property")
	}
	if len(obj.Required) != 1 || obj.Required[0] != "name" {
		t.Error("expected name to be required")
	}

	// Test WithDescription
	s := NewStringSchema().WithDescription("test description")
	if s.Description != "test description" {
		t.Errorf("expected description, got %s", s.Description)
	}

	// Test WithFormat
	s = NewStringSchema().WithFormat("date-time")
	if s.Format != "date-time" {
		t.Errorf("expected format, got %s", s.Format)
	}

	// Test NewArraySchema
	arr := NewArraySchema(NewStringSchema())
	if arr.Type != "array" {
		t.Errorf("expected array type, got %s", arr.Type)
	}
	if arr.Items == nil || arr.Items.Type != "string" {
		t.Error("expected string items")
	}

	// Test NewEnumSchema
	enum := NewEnumSchema([]string{"A", "B", "C"})
	if enum.Type != "string" {
		t.Errorf("expected string type, got %s", enum.Type)
	}
	if len(enum.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(enum.Enum))
	}
}

// TestConverter_Concurrent verifies that the Converter is safe for concurrent use.
// This test should be run with -race to detect data races.
func TestConverter_Concurrent(t *testing.T) {
	// Build a message with nested types to exercise more code paths
	inner := builder.NewMessage("Inner").
		AddField(builder.NewField("value", builder.FieldTypeString()))

	msg := builder.NewMessage("Message").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("count", builder.FieldTypeInt32())).
		AddField(builder.NewField("data", builder.FieldTypeBytes())).
		AddField(builder.NewField("inner", builder.FieldTypeMessage(inner))).
		AddField(builder.NewField("items", builder.FieldTypeMessage(inner)).SetRepeated())

	file := builder.NewFile("test.proto").
		SetPackageName("test").
		AddMessage(inner).
		AddMessage(msg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build: %v", err)
	}

	msgDesc := fd.FindMessage("test.Message")
	if msgDesc == nil {
		t.Fatal("message not found")
	}

	// Use a single converter instance (simulates shared ToolGenerator.converter)
	conv := NewConverter()

	// Run concurrent conversions - this would cause "concurrent map writes" panic
	// before the fix if run with -race
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			schema := conv.MessageToSchema(msgDesc)
			if schema.Type != "object" {
				t.Errorf("expected object type, got %s", schema.Type)
			}
			if schema.Properties["inner"] == nil {
				t.Error("expected inner property")
			}
		}()
	}
	wg.Wait()
}
