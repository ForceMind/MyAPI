package config

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// OptionDiagnosticMaxKeyChars is the largest database option key diagnostics
// will classify. The database reader uses the same character limit so an
// untrusted key is bounded before it enters Go.
const OptionDiagnosticMaxKeyChars = 256

// OptionDiagnosticAssessment contains only stable diagnostic codes. It never
// contains a configuration value or a parser error.
type OptionDiagnosticAssessment struct {
	KeyClass   string
	Validation string
}

// AssessOptionDiagnostic evaluates a registered configuration key without
// reading or changing the registered value. Reflection-based configurations
// are parsed into a zero-value field. A managed map configuration is inspected
// only through DiagnosticSchema, whose zero value carries field names and types
// but no configured value; its own validator is never invoked here, because
// that would read a live runtime generation. A managed configuration without
// that schema stays indeterminate rather than being guessed at.
func (cm *ConfigManager) AssessOptionDiagnostic(key, value string) OptionDiagnosticAssessment {
	if !validOptionDiagnosticKey(key) {
		return OptionDiagnosticAssessment{KeyClass: "malformed_key"}
	}

	prefix, field, dotted := strings.Cut(key, ".")
	if !dotted {
		return OptionDiagnosticAssessment{KeyClass: "legacy_flat"}
	}
	registered := cm.Get(prefix)
	if registered == nil {
		return OptionDiagnosticAssessment{KeyClass: "unknown_prefix"}
	}
	if field == "" || strings.Contains(field, ".") {
		return OptionDiagnosticAssessment{KeyClass: "malformed_key"}
	}
	schema := registered
	if managed, isManaged := registered.(MapConfig); isManaged {
		describable, describes := managed.(DiagnosticSchemaMapConfig)
		if !describes {
			return OptionDiagnosticAssessment{KeyClass: "schema_indeterminate", Validation: "unsupported"}
		}
		schema = describable.DiagnosticSchema()
	}

	typ := reflect.TypeOf(schema)
	for typ != nil && typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return OptionDiagnosticAssessment{KeyClass: "schema_indeterminate", Validation: "unsupported"}
	}

	for index := 0; index < typ.NumField(); index++ {
		fieldType := typ.Field(index)
		if !fieldType.IsExported() {
			continue
		}
		fieldKey := strings.Split(fieldType.Tag.Get("json"), ",")[0]
		if fieldKey == "-" {
			continue
		}
		if fieldKey == "" {
			fieldKey = fieldType.Name
		}
		if fieldKey != field {
			continue
		}
		zeroField := reflect.New(fieldType.Type).Elem()
		if _, err := parseConfigField(zeroField, value); err != nil {
			return OptionDiagnosticAssessment{KeyClass: "registered_field", Validation: diagnosticValidationCode(zeroField.Kind())}
		}
		return OptionDiagnosticAssessment{KeyClass: "registered_field"}
	}
	return OptionDiagnosticAssessment{KeyClass: "unknown_field"}
}

// OptionDiagnosticFieldKind reports the stable storage kind class of a
// registered configuration field: "boolean", "integer", "unsigned_integer",
// "finite_number", "string", "json", or "unsupported". ok is false for keys
// that are not registered fields. The lookup walks the same schema-only path
// as AssessOptionDiagnostic: it never reads a configured value and never
// invokes a configuration validator.
func (cm *ConfigManager) OptionDiagnosticFieldKind(key string) (kind string, ok bool) {
	if !validOptionDiagnosticKey(key) {
		return "", false
	}
	prefix, field, dotted := strings.Cut(key, ".")
	if !dotted || field == "" || strings.Contains(field, ".") {
		return "", false
	}
	registered := cm.Get(prefix)
	if registered == nil {
		return "", false
	}
	schema := registered
	if managed, isManaged := registered.(MapConfig); isManaged {
		describable, describes := managed.(DiagnosticSchemaMapConfig)
		if !describes {
			return "", false
		}
		schema = describable.DiagnosticSchema()
	}
	typ := reflect.TypeOf(schema)
	for typ != nil && typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return "", false
	}
	for index := 0; index < typ.NumField(); index++ {
		fieldType := typ.Field(index)
		if !fieldType.IsExported() {
			continue
		}
		fieldKey := strings.Split(fieldType.Tag.Get("json"), ",")[0]
		if fieldKey == "-" {
			continue
		}
		if fieldKey == "" {
			fieldKey = fieldType.Name
		}
		if fieldKey != field {
			continue
		}
		return optionDiagnosticFieldKindClass(fieldType.Type.Kind()), true
	}
	return "", false
}

func optionDiagnosticFieldKindClass(kind reflect.Kind) string {
	switch kind {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "unsigned_integer"
	case reflect.Float32, reflect.Float64:
		return "finite_number"
	case reflect.String:
		return "string"
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Struct:
		return "json"
	default:
		return "unsupported"
	}
}

func validOptionDiagnosticKey(key string) bool {
	if !utf8.ValidString(key) || key == "" || utf8.RuneCountInString(key) > OptionDiagnosticMaxKeyChars || strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") || strings.Contains(key, "..") {
		return false
	}
	for _, character := range key {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func diagnosticValidationCode(kind reflect.Kind) string {
	switch kind {
	case reflect.Bool:
		return "invalid_boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "invalid_integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "invalid_unsigned_integer"
	case reflect.Float32, reflect.Float64:
		return "invalid_finite_number"
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Struct:
		return "invalid_json"
	default:
		return "unsupported"
	}
}
