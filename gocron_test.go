package gocron

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantHour int
		wantMin  int
		wantSec  int
		wantErr  bool
	}{
		{
			name:     "normal",
			args:     "16:18",
			wantHour: 16,
			wantMin:  18,
			wantErr:  false,
		},
		{
			name:     "normal - no leading hour zeros",
			args:     "6:08",
			wantHour: 6,
			wantMin:  8,
			wantErr:  false,
		},
		{
			name:     "normal_with_second",
			args:     "06:18:01",
			wantHour: 6,
			wantMin:  18,
			wantSec:  1,
			wantErr:  false,
		},
		{
			name:     "normal_with_second - no leading hour zeros",
			args:     "6:08:01",
			wantHour: 6,
			wantMin:  8,
			wantSec:  1,
			wantErr:  false,
		},
		{
			name:     "not_a_number",
			args:     "e:18",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "out_of_range_hour",
			args:     "25:18",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "out_of_range_minute",
			args:     "23:60",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_format",
			args:     "19:18:17:17",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_minute",
			args:     "19:1e",
			wantHour: 19,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_hour",
			args:     "1e:10",
			wantHour: 11,
			wantMin:  0,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHour, gotMin, gotSec, err := parseTime(tt.args)
			if tt.wantErr {
				assert.NotEqual(t, nil, err, tt.args)
				return
			}
			assert.Equal(t, tt.wantHour, gotHour, tt.args)
			assert.Equal(t, tt.wantMin, gotMin, tt.args)
			if tt.wantSec != 0 {
				assert.Equal(t, tt.wantSec, gotSec)
			} else {
				assert.Zero(t, gotSec)
			}
		})
	}
}

func Test_callJobFuncWithParams(t *testing.T) {
	type args struct {
		jobFunc interface{}
		params  []interface{}
	}
	tests := []struct {
		name string
		args args
		err  bool
	}{
		{
			name: "test call func with no args",
			args: args{
				jobFunc: func() {},
				params:  nil,
			},
		},
		{
			name: "test call func with single arg",
			args: args{
				jobFunc: func(arg string) {},
				params:  []interface{}{"test"},
			},
		},
		{
			name: "test call func with wrong arg type",
			args: args{
				jobFunc: func(arg int) {},
				params:  []interface{}{"test"},
			},
			err: true,
		},
		{
			name: "test call func with wrong arg count",
			args: args{
				jobFunc: func(arg int) {},
				params:  []interface{}{},
			},
			err: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := panicFnToErr(func() {
				callJobFuncWithParams(tt.args.jobFunc, tt.args.params)
			})

			if err != nil && !tt.err {
				t.Fatalf("unexpected panic: %s", err.Error())
			}

		})
	}
}

func panicFnToErr(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("func panic")
		}
	}()
	fn()
	return err
}

func Test_getFunctionName(t *testing.T) {
	type args struct {
		fn interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test get function name",
			args: args{
				fn: Test_getFunctionName,
			},
			want: "github.com/go-co-op/gocron.Test_getFunctionName",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, getFunctionName(tt.args.fn), "getFunctionName(%v)", tt.args.fn)
		})
	}
}
