package redislock

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testcontainersredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestEnableDistributedLocking(t *testing.T) {
	ctx := context.Background()
	redisContainer, err := testcontainersredis.RunContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := redisContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	})

	uri, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)

	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
	}

	redisClient := redis.NewClient(&redis.Options{Addr: strings.TrimPrefix(uri, "redis://")})
	l, err := NewRedisLocker(redisClient)
	require.NoError(t, err)

	s1 := gocron.NewScheduler(time.UTC)
	s1.WithDistributedLocker(l)
	_, err = s1.Every("500ms").Do(f, 1)
	require.NoError(t, err)

	s2 := gocron.NewScheduler(time.UTC)
	s2.WithDistributedLocker(l)
	_, err = s2.Every("500ms").Do(f, 2)
	require.NoError(t, err)

	s3 := gocron.NewScheduler(time.UTC)
	s3.WithDistributedLocker(l)
	_, err = s3.Every("500ms").Do(f, 3)
	require.NoError(t, err)

	s1.StartAsync()
	s2.StartAsync()
	s3.StartAsync()

	time.Sleep(1700 * time.Millisecond)

	s1.Stop()
	s2.Stop()
	s3.Stop()
	close(resultChan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 4)
}
