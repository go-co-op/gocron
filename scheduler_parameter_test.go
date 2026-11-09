package gocron

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScheduler_FixedParameterTypes(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	type count int
	n := 1
	pointer := &n
	tests := []struct {
		name string
		task Task
		err  error
	}{
		{"string to any", NewTask(func(any) {}, "value"), nil},
		{"integer pointer", NewTask(func(*int) {}, &n), nil},
		{"nested pointer", NewTask(func(**int) {}, &pointer), nil},
		{"implemented interface", NewTask(func(fmt.Stringer) {}, time.Second), nil},
		{"typed nil pointer", NewTask(func(*int) {}, (*int)(nil)), nil},
		{"receive channel", NewTask(func(<-chan int) {}, make(chan int)), nil},
		{"named integer", NewTask(func(count) {}, count(1)), nil},
		{"variadic fixed pointer", NewTask(func(*int, ...string) {}, &n, "value"), nil},
		{"different struct", NewTask(func(struct{ A int }) {}, struct{ B int }{}), ErrNewJobWrongTypeOfParameters},
		{"different slice", NewTask(func([]int) {}, []string{}), ErrNewJobWrongTypeOfParameters},
		{"different map", NewTask(func(map[string]int) {}, map[string]string{}), ErrNewJobWrongTypeOfParameters},
		{"different function", NewTask(func(func(int)) {}, func(string) {}), ErrNewJobWrongTypeOfParameters},
		{"different pointer", NewTask(func(*int) {}, new(string)), ErrNewJobWrongTypeOfParameters},
		{"unimplemented interface", NewTask(func(fmt.Stringer) {}, struct{}{}), ErrNewJobWrongTypeOfParameters},
		{"convertible integer", NewTask(func(count) {}, 1), ErrNewJobWrongTypeOfParameters},
		{"wrong channel direction", NewTask(func(chan int) {}, make(<-chan int)), ErrNewJobWrongTypeOfParameters},
		{"untyped nil", NewTask(func(any) {}, nil), ErrNewJobWrongTypeOfParameters},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)
			defer func() { require.NoError(t, s.Shutdown()) }()
			_, err := s.NewJob(DurationJob(time.Hour), tt.task)
			require.ErrorIs(t, err, tt.err)
		})
	}
}
