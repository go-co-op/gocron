package gocron

import (
	"errors"
	"testing"
	"time"
)

func TestNewJobVariadicArgumentValidation(t *testing.T) {
	type count int
	tests := []struct {
		name string
		fn   any
		arg  any
	}{
		{"named int for variadic of different named type", func(...count) {}, 1},
		{"wrong slice type", func(...[]int) {}, []string{"x"}},
		{"nil argument for interface variadic", func(...any) {}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewScheduler()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Shutdown() }()
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("NewJob panicked: %v", p)
				}
			}()
			_, err = s.NewJob(DurationJob(time.Hour), NewTask(tc.fn, tc.arg))
			if !errors.Is(err, ErrNewJobWrongTypeOfParameters) {
				t.Errorf("expected ErrNewJobWrongTypeOfParameters, got %v", err)
			}
		})
	}
}
