package server

import (
	"errors"
	"fmt"

	"github.com/go-playground/validator/v10"
)

func ValidateStruct(i any) error {
	v := validator.New()

	err := v.Struct(i)
	if err == nil {
		return nil
	}

	var verr validator.ValidationErrors
	if !errors.As(err, &verr) {
		return fmt.Errorf("validation error: %w", err)
	}

	if len(verr) == 0 {
		return nil
	}

	// return the first error
	firstError := verr[0]

	// if required
	if firstError.Tag() == "required" {
		return fmt.Errorf("field %s is required", firstError.Field())
	}

	// change to "Field `field` is ..."
	return fmt.Errorf("field %s requires %s", firstError.Field(), firstError.Tag()+" "+firstError.Param())
}
