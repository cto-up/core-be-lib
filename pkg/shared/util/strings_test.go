package util

import "testing"

func TestInterpolate(t *testing.T) {
	type args struct {
		template  string
		variables map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test",
			args: args{
				template: `The gopher ${name} is ${days} days old.`,
				variables: map[string]string{
					"name": "Jean",
					"days": "50",
				},
			},
			want: "The gopher Jean is 50 days old.",
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Interpolate(tt.args.template, tt.args.variables); got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}
