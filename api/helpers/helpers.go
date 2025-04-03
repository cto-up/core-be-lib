package helpers

import (
	"fmt"
	"reflect"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

func ErrorResponse(err error) gin.H {
	log.Printf("error: %v\n", err)
	return gin.H{
		"message": err.Error(),
	}
}

func ErrorStringResponse(errMsg string) gin.H {
	return gin.H{
		"message": errMsg,
	}
}

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
