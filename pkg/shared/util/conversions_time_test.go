package util

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
)

func TestToNullable_Time_Generic(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
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
			case *time.Time:
				result = ToNullable(v)
			default:
				t.Fatalf("unsupported type: %T", tt.input)
			}

			assert.Equal(t, tt.expected, result)
		})
	}
}
func TestFromNullableTime(t *testing.T) {
	tests := []struct {
		name         string
		input        interface{}
		expected     interface{}
		compareValue bool
	}{
		// Timestamp tests
		{
			name: "time from present date X",
			input: pgtype.Date{
				Time:  time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
				Valid: true,
			},
			expected: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
		},

		// Date tests
		{
			name:     "time from present date Y",
			input:    pgtype.Date{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
			expected: timePtr(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result interface{}
			switch v := tt.input.(type) {
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
