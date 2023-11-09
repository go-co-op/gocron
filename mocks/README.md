# gocron mocks

## Quick Start

```
go get github.com/go-co-op/gocronmocks/v2
```

write a test
```golang
package main

import (
	"testing"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-co-op/gocronmocks/v2"
	"go.uber.org/mock/gomock"
)

func myFunc(s gocron.Scheduler) {
	s.Start()
	_ = s.Shutdown()
}

func TestMyFunc(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := gocronmocks.NewMockScheduler(ctrl)
	s.EXPECT().Start().Times(1)
	s.EXPECT().Shutdown().Times(1).Return(nil)

	myFunc(s)
}

```
