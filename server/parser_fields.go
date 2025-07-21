package server

import (
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type typeParser func(value string) (any, error)

var typeParsers = map[string]typeParser{
	"string": func(v string) (any, error) { return v, nil },
	"bool":   func(v string) (any, error) { return strconv.ParseBool(v) },
	"int":    func(v string) (any, error) { return strconv.Atoi(v) },
	"int32": func(v string) (any, error) {
		i, err := strconv.ParseInt(v, 10, 32)
		return int32(i), err
	},
	"int64": func(v string) (any, error) {
		return strconv.ParseInt(v, 10, 64)
	},
	"float32": func(v string) (any, error) {
		f, err := strconv.ParseFloat(v, 32)
		return float32(f), err
	},
	"float64": func(v string) (any, error) {
		return strconv.ParseFloat(v, 64)
	},
	"UUID": func(v string) (any, error) { // Note: reflect.Type.Name() for uuid.UUID is "UUID"
		return uuid.Parse(v)
	},
	"Time": func(v string) (any, error) { // Note: reflect.Type.Name() for time.Time is "Time"
		return time.Parse(time.RFC3339, v)
	},
}

// parseField is a helper function that parses the field value which is a string,
// and returns the value as the correct type
func parseField(field reflect.StructField, value string) (any, error) {
	// check default value
	if value == "" {
		if defaultValue := field.Tag.Get("default"); defaultValue != "" {
			value = defaultValue
		} else if field.Type.Kind() == reflect.Ptr {
			// For pointer types with no value and no default, return nil.
			return nil, nil
		}
	}

	// 3. Determine the type and if it's a pointer.
	fieldType := field.Type
	isPointer := fieldType.Kind() == reflect.Ptr
	if isPointer {
		// If it's a pointer, get the underlying type (e.g., *int -> int).
		fieldType = fieldType.Elem()
	}

	// 4. Look up the correct parser from our registry.
	parser, ok := typeParsers[fieldType.Name()]
	if !ok {
		// If no specific parser is found, default to treating it as a string.
		parser = typeParsers["string"]
	}

	// 5. Execute the parser.
	parsedValue, err := parser(value)
	if err != nil {
		return nil, err
	}

	// 6. If the original type was a pointer, return the address of the parsed value.
	if isPointer {
		// To return a pointer, we need to create a new pointer to the value.
		val := reflect.ValueOf(parsedValue)
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		return ptr.Interface(), nil
	}

	return parsedValue, nil
}

func parseFields(field reflect.StructField, values []string) (any, error) {
	if len(values) == 0 {
		return nil, nil // No values to parse
	}

	// If the field is a slice, we need to parse each value into the slice type.
	if field.Type.Kind() == reflect.Slice {
		sliceType := field.Type.Elem()
		slice := reflect.MakeSlice(field.Type, 0, len(values))

		for _, v := range values {
			value, err := parseField(field, v)
			if err != nil {
				return nil, err
			}
			if value != nil {
				slice = reflect.Append(slice, reflect.ValueOf(value).Convert(sliceType))
			}
		}
		return slice.Interface(), nil
	}

	// If it's not a slice, just parse the first value.
	return parseField(field, values[0])
}
