package tools

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
)

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// SchemaFor describes how T looks as JSON, so a tool's output schema can't drift from the
// type it returns. A type with its own JSON encoding, such as a time, has no known shape,
// so it's left without a type.
func SchemaFor[T any]() map[string]any {
	return schemaOf(reflect.TypeFor[T](), map[reflect.Type]bool{})
}

func schemaOf(t reflect.Type, parents map[reflect.Type]bool) map[string]any {
	if t.Kind() == reflect.Pointer {
		return schemaOf(t.Elem(), parents)
	}
	if t.Implements(jsonMarshaler) || t.Implements(textMarshaler) ||
		reflect.PointerTo(t).Implements(jsonMarshaler) || reflect.PointerTo(t).Implements(textMarshaler) {
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Map:
		return map[string]any{"type": "object"}
	case reflect.Slice, reflect.Array:
		// Bytes encode as a base64 string.
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": schemaOf(t.Elem(), parents)}
	case reflect.Struct:
		// A type that contains itself stops at the first repeat.
		if parents[t] {
			return map[string]any{"type": "object"}
		}
		parents[t] = true
		defer delete(parents, t)
		props := map[string]any{}
		addFields(t, props, parents)
		return map[string]any{"type": "object", "properties": props}
	}
	return map[string]any{}
}

// addFields follows encoding/json's rules for names, skipped fields and embedded structs.
func addFields(t reflect.Type, props map[string]any, parents map[reflect.Type]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "-" {
			continue
		}
		if f.Anonymous && tag == "" {
			embedded := f.Type
			if embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				addFields(embedded, props, parents)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		name := tag
		if name == "" {
			name = f.Name
		}
		props[name] = schemaOf(f.Type, parents)
	}
}
