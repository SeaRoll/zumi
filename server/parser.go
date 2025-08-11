package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// ParseRequest parses the HTTP request and populates the provided struct with the data from the request.
// It supports parsing from context, headers, path parameters, query parameters, and JSON body.
//
// Example usage:
//
//	type MyRequest struct {
//	    ID   int       `ctx:"id"`
//	    Name string    `header:"X-Name"`
//	    Age  int       `path:"age"`
//	    Tags []string  `query:"tags"`
//	    Data MyData    `body:"json"`
//	 }
//
//	 var req MyRequest
//	 err := ParseRequest(r, &req)
//
//	 if err != nil {
//	    http.Error(w, err.Error(), http.StatusBadRequest)
//	    return
//	 }
func ParseRequest(r *http.Request, req any) error {
	ctx := r.Context()
	typ := reflect.TypeOf(req).Elem()
	val := reflect.ValueOf(req).Elem()

	for i := range typ.NumField() {
		field := typ.Field(i)

		// extract context
		tagName := field.Tag.Get("ctx")
		if tagName != "" {
			val.Field(i).Set(reflect.ValueOf(ctx))
			continue
		}

		tagName = field.Tag.Get("header")
		if tagName != "" {
			// make use of the parseField helper function
			value, err := parseField(field, r.Header.Get(tagName))
			if err != nil {
				return fmt.Errorf("error parsing header field %s: %w", tagName, err)
			}

			if value != nil {
				val.Field(i).Set(reflect.ValueOf(value))
			}

			continue
		}

		tagName = field.Tag.Get("path")
		if tagName != "" {
			value, err := parseField(field, r.PathValue(tagName))
			if err != nil {
				return fmt.Errorf("error parsing path field %s: %w", tagName, err)
			}

			if value != nil {
				val.Field(i).Set(reflect.ValueOf(value))
			}

			continue
		}

		tagName = field.Tag.Get("query")
		if tagName != "" {
			queryValues := r.URL.Query()[tagName]

			value, err := parseFields(field, queryValues)
			if err != nil {
				return fmt.Errorf("error parsing query field %s: %w", tagName, err)
			}

			if value != nil {
				val.Field(i).Set(reflect.ValueOf(value))
			}

			continue
		}

		tagName = field.Tag.Get("body")
		if tagName == "json" {
			body := reflect.New(field.Type).Interface()

			err := json.NewDecoder(r.Body).Decode(body)
			if err != nil {
				return fmt.Errorf("error parsing body field %s: %w", tagName, err)
			}

			val.Field(i).Set(reflect.ValueOf(body).Elem())

			continue
		}

		// check if the field is a struct
		if field.Type.Kind() == reflect.Struct {
			// recursively parse the struct
			err := ParseRequest(r, val.Field(i).Addr().Interface())
			if err != nil {
				return fmt.Errorf("error parsing struct field %s: %w", field.Name, err)
			}
		}
	}

	err := ValidateStruct(req)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	return nil
}
