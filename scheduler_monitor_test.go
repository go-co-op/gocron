package gocron

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSchedulerMonitor is a test implementation
type mockSchedulerMonitor struct {
	mu                    sync.Mutex
	schedulerStartedCount int
	schedulerStoppedCount int
}

// SchedulerStarted increments the started count
func (m *mockSchedulerMonitor) SchedulerStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedulerStartedCount++
}

// SchedulerStopped increments the stopped count
func (m *mockSchedulerMonitor) SchedulerStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedulerStoppedCount++
}

// getSchedulerStartedCount returns the count of SchedulerStarted calls
func (m *mockSchedulerMonitor) getSchedulerStartedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.schedulerStartedCount
}

// getSchedulerStoppedCount returns the count of SchedulerStopped calls
func (m *mockSchedulerMonitor) getSchedulerStoppedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.schedulerStoppedCount
}

// Test that SchedulerStopped is called
func TestSchedulerMonitor_SchedulerStopped(t *testing.T) {
	monitor := &mockSchedulerMonitor{}
	s, err := NewScheduler(WithSchedulerMonitor(monitor))
	require.NoError(t, err)

	s.Start()
	time.Sleep(10 * time.Millisecond)

	err = s.Shutdown()
	require.NoError(t, err)

	// verify SchedulerStopped was called exactly once
	assert.Equal(t, 1, monitor.getSchedulerStoppedCount())
}

// Test multiple start/stop cycles
func TestSchedulerMonitor_MultipleStartStopCycles(t *testing.T) {
	monitor := &mockSchedulerMonitor{}
	s, err := NewScheduler(WithSchedulerMonitor(monitor))
	require.NoError(t, err)

	// first cycle
	s.Start()
	time.Sleep(10 * time.Millisecond)
	err = s.Shutdown()
	require.NoError(t, err)

	// second cycle
	s.Start()
	time.Sleep(10 * time.Millisecond)
	err = s.Shutdown()
	require.NoError(t, err)

	// verifying counts
	assert.Equal(t, 2, monitor.getSchedulerStartedCount())
	assert.Equal(t, 2, monitor.getSchedulerStoppedCount())
}

// Test that start and stop are called in correct order
func TestSchedulerMonitor_StartStopOrder(t *testing.T) {
	events := []string{}
	var mu sync.Mutex

	monitor := &orderTrackingMonitor{
		onStart: func() {
			mu.Lock()
			events = append(events, "started")
			mu.Unlock()
		},
		onStop: func() {
			mu.Lock()
			events = append(events, "stopped")
			mu.Unlock()
		},
	}

	s, err := NewScheduler(WithSchedulerMonitor(monitor))
	require.NoError(t, err)

	s.Start()
	time.Sleep(10 * time.Millisecond)
	s.Shutdown()
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"started", "stopped"}, events)
}

// Helper monitor for tracking order
type orderTrackingMonitor struct {
	onStart func()
	onStop  func()
}

func (m *orderTrackingMonitor) SchedulerStarted() {
	if m.onStart != nil {
		m.onStart()
	}
}

func (m *orderTrackingMonitor) SchedulerStopped() {
	if m.onStop != nil {
		m.onStop()
	}
}

// Test that stopped is not called if scheduler wasn't started
func TestSchedulerMonitor_StoppedNotCalledIfNotStarted(t *testing.T) {
	monitor := &mockSchedulerMonitor{}
	s, err := NewScheduler(WithSchedulerMonitor(monitor))
	require.NoError(t, err)
	err = s.Shutdown()
	if err != nil {
		t.Logf("Shutdown returned error as expected: %v", err)
	}

	// Depending on implementation, this might return an error
	// But stopped should not be called
	assert.Equal(t, 0, monitor.getSchedulerStoppedCount())
}
