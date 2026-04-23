package helpers

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// ValidateUUID implements validator.CustomTypeFunc
func CustomTypeUUID(field reflect.Value) interface{} {
	if valuer, ok := field.Interface().(uuid.UUID); ok {
		fmt.Println(valuer.String())
		return valuer.String()
	} else {
		return "XX"
	}
	//return nil
}

var ValidateUUID validator.Func = func(fl validator.FieldLevel) bool {
	return true
}
