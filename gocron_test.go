package gocron

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatTime(t *testing.T) {
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
			name:     "normal_with_second",
			args:     "6:18:01",
			wantHour: 6,
			wantMin:  18,
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
			gotHour, gotMin, gotSec, err := formatTime(tt.args)
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

type lockerMock struct {
	cache map[string]struct{}
	l     sync.Mutex
}

func (l *lockerMock) Lock(key string) (bool, error) {
	l.l.Lock()
	defer l.l.Unlock()
	if _, ok := l.cache[key]; ok {
		return false, nil
	}
	l.cache[key] = struct{}{}
	return true, nil
}

func (l *lockerMock) Unlock(key string) error {
	l.l.Lock()
	defer l.l.Unlock()
	delete(l.cache, key)
	return nil
}

func TestSetLocker(t *testing.T) {
	if locker != nil {
		t.Fail()
		t.Log("Expected locker to not be set by default")
	}

	SetLocker(&lockerMock{})

	if locker == nil {
		t.Fail()
		t.Log("Expected locker to be set")
	}
}

type lockerResult struct {
	key   string
	cycle int
	s, e  time.Time
}

func TestLocker(t *testing.T) {
	l := sync.Mutex{}

	result := make([]lockerResult, 0)
	task := func(key string, i int) {
		s := time.Now()
		time.Sleep(time.Millisecond * 100)
		e := time.Now()
		l.Lock()
		result = append(result, lockerResult{
			key:   key,
			cycle: i,
			s:     s,
			e:     e,
		})
		l.Unlock()
	}

	SetLocker(&lockerMock{
		make(map[string]struct{}),
		sync.Mutex{},
	})

	for i := 0; i < 5; i++ {
		s1 := NewScheduler(time.UTC)
		s1.Every(1).Seconds().Lock().Do(task, "A", i)

		s2 := NewScheduler(time.UTC)
		s2.Every(1).Seconds().Lock().Do(task, "B", i)

		s3 := NewScheduler(time.UTC)
		s3.Every(1).Seconds().Lock().Do(task, "C", i)

		stop1 := s1.Start()
		stop2 := s2.Start()
		stop3 := s3.Start()

		time.Sleep(time.Millisecond * 100)

		close(stop1)
		close(stop2)
		close(stop3)

		for i := 0; i < len(result)-1; i++ {
			for j := i + 1; j < len(result); j++ {
				iBefJ := result[i].s.Before(result[j].s) && result[i].e.Before(result[j].s)
				jBefI := result[j].s.Before(result[i].s) && result[j].e.Before(result[i].s)
				if !iBefJ && !jBefI {
					t.Fatalf("\n2 operations ran concurrently:\n%s\n%d\n%s\n%s\n**********\n%s\n%d\n%s\n%s\n",
						result[i].key, result[i].cycle, result[i].s, result[i].e,
						result[j].key, result[j].cycle, result[j].s, result[j].e)
				}
			}
		}
	}
}
