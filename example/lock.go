package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jasonlvhit/gocron"

	"github.com/go-redis/redis"
)

// Run a Redis instance with Docker: docker run --rm -tid -p 6379:6379 redis:alpine

func lockedTask(name string) {
	fmt.Printf("Hello, %s!\n", name)

	t := time.NewTicker(time.Millisecond * 100)
	c := make(chan struct{})
	time.AfterFunc(time.Second*5, func() {
		close(c)
	})

	for {
		select {
		case <-t.C:
			fmt.Print(".")
		case <-c:
			fmt.Println()
			return
		}
	}
}

// locker implementation with Redis
type locker struct {
	cache *redis.Client
}

func (s *locker) Lock(key string) (success bool, err error) {
	res, err := s.cache.SetNX(key, time.Now().String(), time.Second*15).Result()
	if err != nil {
		return false, err
	}
	return res, nil
}

func (s *locker) Unlock(key string) error {
	return s.cache.Del(key).Err()
}

// Run the example in different terminals,
// passing a different name parameter to each
func main() {
	// Get a locker
	l := &locker{
		redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		}),
	}

	// Make locker available for the cron jobs
	gocron.SetLocker(l)

	arg := "Some Name"
	args := os.Args[1:]
	if len(args) > 0 {
		arg = args[0]
	}

	gocron.Every(1).Second().Lock().Do(lockedTask, arg)
	<-gocron.Start()
}
