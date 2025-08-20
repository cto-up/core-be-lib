package util

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oapi-codegen/runtime/types"
)

// NullableTypes is a type constraint that limits the allowed pointer types
type NullableTypes interface {
	~string | ~int | ~int32 | ~int64 | ~float32 | ~float64 | ~bool | time.Time
}

// ToNullable converts a pointer of a basic type to its corresponding pgtype nullable type
func ToNullable[T NullableTypes](value *T) any {
	if value == nil {
		return getNullType[T]()
	}
	return getPresentType(value)
}

// Individual typed functions for when you need specific return types
func ToNullableUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{Valid: false}
	}
	uuid := pgtype.UUID{
		Bytes: *value,
		Valid: true,
	}
	return uuid
}

func ToNullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func ToNullableInt4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func ToNullableNumericFromInt(value *int64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{Valid: false}
	}

	return pgtype.Numeric{Int: big.NewInt(*value), Valid: true}
}

func ToNullableNumericFromFloat(f *float32) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{
			Valid: false,
		}
	}

	// Convert float32 to string to avoid precision loss
	floatStr := fmt.Sprintf("%f", *f)

	// Parse the string into a big.Int and determine the exponent
	// For example, 123.456 becomes 123456 with an exponent of -3
	var intVal big.Int
	var exp int32

	// Split the string into integer and fractional parts
	parts := strings.Split(floatStr, ".")
	if len(parts) == 1 {
		// No fractional part
		intVal.SetString(parts[0], 10)
		exp = 0
	} else {
		// Combine integer and fractional parts into a single integer
		combined := parts[0] + parts[1]
		intVal.SetString(combined, 10)
		exp = -int32(len(parts[1])) // Exponent is negative for fractional parts
	}

	return pgtype.Numeric{
		Int:   &intVal,
		Exp:   exp,
		Valid: true,
	}
}

func ToNullableInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func ToNullableFloat4(value *float32) pgtype.Float4 {
	if value == nil {
		return pgtype.Float4{Valid: false}
	}
	return pgtype.Float4{Float32: *value, Valid: true}
}

func ToNullableFloat8(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{Valid: false}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func ToNullableBool(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{Valid: false}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}
func ToNullableDate(value *types.Date) pgtype.Date {
	if value == nil {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: value.Time, Valid: true}
}

func ToNullableTimestamp(value *time.Time) pgtype.Timestamp {
	if value == nil {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *value, Valid: true}
}
func ToNullableTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func ToNullableSlice(ptr *[]string) []string {
	if ptr == nil {
		return []string{}
	}
	return *ptr
}

func ToNullableNumericDecimal(value float64, nberOfDecimal int) (pgtype.Numeric, error) {
	r := new(big.Rat).SetFloat64(value)
	if r == nil {
		fmt.Println("Could not convert float64 to big.Rat")
		return pgtype.Numeric{}, nil
	}

	scale := new(big.Rat).SetFloat64(math.Pow10(nberOfDecimal))
	r.Mul(r, scale) // Scale up

	// Now r is scaled, but still a *big.Rat (fraction)
	// We need an integer: r.Num() / r.Denom()

	// Divide numerator by denominator to get the final integer
	intVal := new(big.Int).Quo(r.Num(), r.Denom())
	scoreDB := pgtype.Numeric{
		Int:   intVal,
		Exp:   int32(-nberOfDecimal),
		Valid: true,
	}
	return scoreDB, nil
}
func FromNullableDecimal(num pgtype.Numeric) float32 {
	if !num.Valid {
		return 0
	}

	// Create a big.Rat from the Numeric's components
	r := new(big.Rat).SetFrac(num.Int, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-num.Exp)), nil))

	// Convert to float64
	f, exact := r.Float32()
	if !exact {
		// The conversion wasn't exact, but we'll still return the approximate value
		// You might want to handle this case differently depending on your needs
		return 0
	}

	return f
}

// Helper functions for the generic version
func getNullType[T NullableTypes]() any {
	var zero T
	switch any(zero).(type) {
	case string:
		return pgtype.Text{Valid: false}
	case int, int32:
		return pgtype.Int4{Valid: false}
	case int64:
		return pgtype.Int8{Valid: false}
	case float32:
		return pgtype.Float4{Valid: false}
	case float64:
		return pgtype.Float8{Valid: false}
	case bool:
		return pgtype.Bool{Valid: false}
	case time.Time:
		return pgtype.Timestamp{Valid: false}
	default:
		panic("unsupported type")
	}
}

func getPresentType[T NullableTypes](value *T) any {
	switch v := any(*value).(type) {
	case string:
		return pgtype.Text{String: v, Valid: true}
	case int:
		return pgtype.Int4{Int32: int32(v), Valid: true}
	case int32:
		return pgtype.Int4{Int32: v, Valid: true}
	case int64:
		return pgtype.Int8{Int64: v, Valid: true}
	case float32:
		return pgtype.Float4{Float32: v, Valid: true}
	case float64:
		return pgtype.Float8{Float64: v, Valid: true}
	case bool:
		return pgtype.Bool{Bool: v, Valid: true}
	case time.Time:
		return pgtype.Timestamp{Time: v, Valid: true}
	default:
		panic("unsupported type")
	}
}

// PgNullableTypes is a type constraint that limits the allowed pgtype inputs
type PgNullableTypes interface {
	pgtype.Text | pgtype.Int4 | pgtype.Int8 | pgtype.Float4 | pgtype.Float8 |
		pgtype.Bool | pgtype.Timestamp | pgtype.Date
}

// ToJSON converts an interface to a JSON string.
func ToJSON(v interface{}) []byte {
	bytes, err := json.Marshal(v)
	if err != nil {
		return []byte("")
	}
	return bytes
}
func FromJSON[T any](data []byte) T {
	var result T
	if data == nil {
		return result
	}
	err := json.Unmarshal(data, &result)
	if err != nil {
		return result
	}

	return result
}

// FromNullable converts pgtype nullable types to pointers of Go primitive types
func FromNullable[T any, P PgNullableTypes](value P) *T {
	switch v := any(value).(type) {
	case pgtype.UUID:
		if v.Valid {
			uuid, _ := uuid.FromBytes(v.Bytes[:])
			return any(&uuid).(*T)
		}
	case pgtype.Text:
		if v.Valid {
			return any(&v.String).(*T)
		}
	case pgtype.Int4:
		if v.Valid {
			switch any(*new(T)).(type) {
			case int:
				i := int(v.Int32)
				return any(&i).(*T)
			case int32:
				return any(&v.Int32).(*T)
			}
		}
	case pgtype.Int8:
		if v.Valid {
			i := int64(v.Int64)
			return any(&i).(*T)
		}
	case pgtype.Float4:
		if v.Valid {
			switch any(*new(T)).(type) {
			case float32:
				return any(&v.Float32).(*T)
			case float64:
				f := float64(v.Float32)
				return any(&f).(*T)
			}
		}
	case pgtype.Float8:
		if v.Valid {
			f := float64(v.Float64)
			return any(&f).(*T)
		}
	case pgtype.Bool:
		if v.Valid {
			return any(&v.Bool).(*T)
		}
	case pgtype.Timestamp:
		if v.Valid {
			return any(&v.Time).(*T)
		}
	case pgtype.Date:
		if v.Valid {
			return any(&v.Time).(*T)
		}
	}
	return nil
}

// Type-specific versions for better type safety

func FromNullableText(value pgtype.Text) *string {
	if value.Valid {
		return &value.String
	}
	return nil
}

func FromNullableInt4(value pgtype.Int4) *int32 {
	if value.Valid {
		return &value.Int32
	}
	return nil
}

func FromNullableInt8(value pgtype.Int8) *int64 {
	if value.Valid {
		return &value.Int64
	}
	return nil
}

func FromNullableFloat4(value pgtype.Float4) *float32 {
	if value.Valid {
		return &value.Float32
	}
	return nil
}

func FromNullableFloat8(value pgtype.Float8) *float64 {
	if value.Valid {
		return &value.Float64
	}
	return nil
}

func FromNullableBool(value pgtype.Bool) *bool {
	if value.Valid {
		return &value.Bool
	}
	return nil
}

func FromNullableTimestamp(value pgtype.Timestamp) *time.Time {
	if value.Valid {
		return &value.Time
	}
	return nil
}
func FromNullableTimestamptz(value pgtype.Timestamptz) *time.Time {
	if value.Valid {
		return &value.Time
	}
	return nil
}

// Converts a pgtype.UUID to a *uuid.UUID
func FromNullableUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	uuid, _ := uuid.FromBytes(value.Bytes[:])
	return &uuid
}
