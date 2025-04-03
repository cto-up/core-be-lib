package util

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestToNullable_Generic(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		// String tests
		{
			name:     "string pointer",
			input:    stringPtr("test"),
			expected: pgtype.Text{String: "test", Valid: true},
		},
		{
			name:     "nil string pointer",
			input:    (*string)(nil),
			expected: pgtype.Text{Valid: false},
		},

		// Int tests
		{
			name:     "int pointer",
			input:    intPtr(42),
			expected: pgtype.Int4{Int32: 42, Valid: true},
		},
		{
			name:     "nil int pointer",
			input:    (*int)(nil),
			expected: pgtype.Int4{Valid: false},
		},

		// Int32 tests
		{
			name:     "int32 pointer",
			input:    int32Ptr(42),
			expected: pgtype.Int4{Int32: 42, Valid: true},
		},

		// Int64 tests
		{
			name:     "int64 pointer",
			input:    int64Ptr(42),
			expected: pgtype.Int8{Int64: 42, Valid: true},
		},
		{
			name:     "nil int64 pointer",
			input:    (*int64)(nil),
			expected: pgtype.Int8{Valid: false},
		},

		// Float32 tests
		{
			name:     "float32 pointer",
			input:    float32Ptr(3.14),
			expected: pgtype.Float4{Float32: 3.14, Valid: true},
		},

		// Float64 tests
		{
			name:     "float64 pointer",
			input:    float64Ptr(3.14159),
			expected: pgtype.Float8{Float64: 3.14159, Valid: true},
		},
		{
			name:     "nil float64 pointer",
			input:    (*float64)(nil),
			expected: pgtype.Float8{Valid: false},
		},

		// Bool tests
		{
			name:     "bool pointer (true)",
			input:    boolPtr(true),
			expected: pgtype.Bool{Bool: true, Valid: true},
		},
		{
			name:     "bool pointer (false)",
			input:    boolPtr(false),
			expected: pgtype.Bool{Bool: false, Valid: true},
		},
		{
			name:     "nil bool pointer",
			input:    (*bool)(nil),
			expected: pgtype.Bool{Valid: false},
		},

		// Time tests
		{
			name:     "time pointer",
			input:    timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
			expected: pgtype.Timestamp{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		},
		{
			name:     "nil time pointer",
			input:    (*time.Time)(nil),
			expected: pgtype.Timestamp{Valid: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			switch v := tt.input.(type) {
			case *string:
				result = ToNullable(v)
			case *int:
				result = ToNullable(v)
			case *int32:
				result = ToNullable(v)
			case *int64:
				result = ToNullable(v)
			case *float32:
				result = ToNullable(v)
			case *float64:
				result = ToNullable(v)
			case *bool:
				result = ToNullable(v)
			case *time.Time:
				result = ToNullable(v)
			default:
				t.Fatalf("unsupported type: %T", tt.input)
			}

			assert.Equal(t, tt.expected, result)
		})
	}
}
func TestFromNullable(t *testing.T) {
	tests := []struct {
		name         string
		input        interface{}
		expected     interface{}
		compareValue bool
	}{
		// Text tests
		{
			name:     "string from present text",
			input:    pgtype.Text{String: "test", Valid: true},
			expected: stringPtr("test"),
		},
		{
			name:     "nil from null text",
			input:    pgtype.Text{Valid: false},
			expected: (*string)(nil),
		},

		// Int4 tests
		{
			name:     "int32 from present int4",
			input:    pgtype.Int4{Int32: 42, Valid: true},
			expected: int32Ptr(42),
		},
		{
			name:     "int from present int4",
			input:    pgtype.Int4{Int32: 42, Valid: true},
			expected: intPtr(42),
		},
		{
			name:     "nil from null int4",
			input:    pgtype.Int4{Valid: false},
			expected: (*int32)(nil),
		},

		// Int8 tests
		{
			name:     "int64 from present int8",
			input:    pgtype.Int8{Int64: 42, Valid: true},
			expected: int64Ptr(42),
		},

		{
			name:         "float32 from present float4",
			input:        pgtype.Float4{Float32: 3.14, Valid: true},
			expected:     float32Ptr(3.14),
			compareValue: true,
		},
		{
			name:         "float64 from present float4",
			input:        pgtype.Float4{Float32: 3.14, Valid: true},
			expected:     float64Ptr(3.14),
			compareValue: true,
		},

		// Bool tests
		{
			name:     "true from present bool",
			input:    pgtype.Bool{Bool: true, Valid: true},
			expected: boolPtr(true),
		},
		{
			name:     "false from present bool",
			input:    pgtype.Bool{Bool: false, Valid: true},
			expected: boolPtr(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			switch v := tt.input.(type) {
			case pgtype.Text:
				result = FromNullable[string](v)
			case pgtype.Int4:
				switch tt.expected.(type) {
				case *int32:
					result = FromNullable[int32](v)
				case *int:
					result = FromNullable[int](v)
				}
			case pgtype.Int8:
				result = FromNullable[int64](v)
			case pgtype.Float4:
				switch tt.expected.(type) {
				case *float32:
					result = FromNullable[float32](v)
				case *float64:
					result = FromNullable[float64](v)
				}
			case pgtype.Float8:
				result = FromNullable[float64](v)
			case pgtype.Bool:
				result = FromNullable[bool](v)
			case pgtype.Timestamp:
				result = FromNullable[time.Time](v)
			case pgtype.Date:
				result = FromNullable[time.Time](v)
			}

			if tt.expected == nil {
				assert.Nil(t, result)
			} else if tt.compareValue {
				// For float comparisons, dereference the pointers
				switch expected := tt.expected.(type) {
				case float64:
					if resultPtr, ok := result.(*float64); ok {
						assert.InDelta(t, expected, *resultPtr, 0.0001, "values should be nearly equal")
					} else {
						t.Errorf("expected *float64, got %T", result)
					}
					// Add other value types if needed
				}
			} else {
				// Default pointer comparison
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFromNullable_InvalidCombinations(t *testing.T) {
	// These test cases verify that invalid type combinations return nil
	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{
			name:  "request int from text",
			input: pgtype.Text{String: "123", Valid: true},
			want:  (*int)(nil),
		},
		{
			name:  "request string from int4",
			input: pgtype.Int4{Int32: 42, Valid: true},
			want:  (*string)(nil),
		},
		{
			name:  "request bool from timestamp",
			input: pgtype.Timestamp{Time: time.Now(), Valid: true},
			want:  (*bool)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			switch v := tt.input.(type) {
			case pgtype.Text:
				result = FromNullable[int](v)
			case pgtype.Int4:
				result = FromNullable[string](v)
			case pgtype.Timestamp:
				result = FromNullable[bool](v)
			}
			assert.Nil(t, result)
		})
	}
}

// Helper functions to create pointers for test values
func stringPtr(s string) *string     { return &s }
func intPtr(i int) *int              { return &i }
func int32Ptr(i int32) *int32        { return &i }
func int64Ptr(i int64) *int64        { return &i }
func float32Ptr(f float32) *float32  { return &f }
func float64Ptr(f float64) *float64  { return &f }
func boolPtr(b bool) *bool           { return &b }
func timePtr(t time.Time) *time.Time { return &t }
