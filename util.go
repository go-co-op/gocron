package gocron

import (
	"context"
	"reflect"
	"sync"
	"time"

	"fmt"
	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func callJobFuncWithParams(jobFunc any, params ...any) error {
	if jobFunc == nil {
		return nil
	}
	f := reflect.ValueOf(jobFunc)
	if f.IsZero() {
		return nil
	}
	if params == nil {
		params = []any{}
	}
	// 检查参数个数是否匹配
	if len(params) != f.Type().NumIn() {
		return fmt.Errorf("parameter count mismatch")
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		if param == nil {
			// 如果参数是 nil，需要确认函数参数类型
			paramType := f.Type().In(k)
			if paramType.Kind() == reflect.Ptr || paramType.Kind() == reflect.Interface {
				// 针对指针或接口类型的 nil 参数，使用 reflect.Zero 创建 reflect.Value
				in[k] = reflect.Zero(paramType)
			} else {
				// 非指针和接口类型不支持 nil
				return fmt.Errorf("parameter %d is nil but requires non-nil value of type %s", k, paramType)
			}
		} else {
			in[k] = reflect.ValueOf(param)
		}
	}
	returnValues := f.Call(in)
	for _, val := range returnValues {
		i := val.Interface()
		if err, ok := i.(error); ok {
			return err
		}
	}
	return nil
}

func callJobFuncHasReturnWithParams(jobFunc any, params ...any) interface{} {
	if jobFunc == nil {
		return nil
	}
	f := reflect.ValueOf(jobFunc)
	if f.IsZero() {
		return nil
	}
	if params == nil {
		params = []any{}
	}
	// 检查参数个数是否匹配
	if len(params) != f.Type().NumIn() {
		return fmt.Errorf("parameter count mismatch")
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		if param == nil {
			// 如果参数是 nil，需要确认函数参数类型
			paramType := f.Type().In(k)
			if paramType.Kind() == reflect.Ptr || paramType.Kind() == reflect.Interface {
				// 针对指针或接口类型的 nil 参数，使用 reflect.Zero 创建 reflect.Value
				in[k] = reflect.Zero(paramType)
			} else {
				// 非指针和接口类型不支持 nil
				return fmt.Errorf("parameter %d is nil but requires non-nil value of type %s", k, paramType)
			}
		} else {
			in[k] = reflect.ValueOf(param)
		}
	}
	returnValues := f.Call(in)
	if len(returnValues) > 0 {
		return returnValues[0].Interface()
	}
	return nil
}

func requestJob(id uuid.UUID, ch chan jobOutRequest) *internalJob {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return requestJobCtx(ctx, id, ch)
}

func requestJobCtx(ctx context.Context, id uuid.UUID, ch chan jobOutRequest) *internalJob {
	resp := make(chan internalJob, 1)
	select {
	case ch <- jobOutRequest{
		id:      id,
		outChan: resp,
	}:
	case <-ctx.Done():
		return nil
	}
	var j internalJob
	select {
	case <-ctx.Done():
		return nil
	case jobReceived := <-resp:
		j = jobReceived
	}
	return &j
}

func removeSliceDuplicatesInt(in []int) []int {
	m := make(map[int]struct{})

	for _, i := range in {
		m[i] = struct{}{}
	}
	return maps.Keys(m)
}

func convertAtTimesToDateTime(atTimes AtTimes, location *time.Location) ([]time.Time, error) {
	if atTimes == nil {
		return nil, errAtTimesNil
	}
	var atTimesDate []time.Time
	for _, a := range atTimes() {
		if a == nil {
			return nil, errAtTimeNil
		}
		at := a()
		if at.hours > 23 {
			return nil, errAtTimeHours
		} else if at.minutes > 59 || at.seconds > 59 {
			return nil, errAtTimeMinSec
		}
		atTimesDate = append(atTimesDate, at.time(location))
	}
	slices.SortStableFunc(atTimesDate, ascendingTime)
	return atTimesDate, nil
}

func ascendingTime(a, b time.Time) int {
	return a.Compare(b)
}

type waitGroupWithMutex struct {
	wg sync.WaitGroup
	mu sync.Mutex
}

func (w *waitGroupWithMutex) Add(delta int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Add(delta)
}

func (w *waitGroupWithMutex) Done() {
	w.wg.Done()
}

func (w *waitGroupWithMutex) Wait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Wait()
}
