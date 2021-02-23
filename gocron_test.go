package gocron

import (
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
