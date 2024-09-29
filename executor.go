package gocron

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/google/uuid"
)

type executor struct {
	// context used for shutting down
	ctx context.Context
	// cancel used by the executor to signal a stop of it's functions
	cancel context.CancelFunc
	// clock used for regular time or mocking time
	clock clockwork.Clock
	// the executor's logger
	logger Logger

	// receives jobs scheduled to execute
	jobsIn chan jobIn
	// sends out jobs for rescheduling
	jobsOutForRescheduling chan jobIn
	// sends out jobs once completed
	jobsOutCompleted chan jobIn
	// used to request jobs from the scheduler
	jobOutRequest chan jobOutRequest

	// used by the executor to receive a stop signal from the scheduler
	stopCh chan struct{}
	// the timeout value when stopping
	stopTimeout time.Duration
	// used to signal that the executor has completed shutdown
	done chan error

	// runners for any singleton type jobs
	// map[uuid.UUID]singletonRunner
	singletonRunners *sync.Map
	// config for limit mode
	limitMode *limitModeConfig
	// the elector when running distributed instances
	elector Elector
	// the locker when running distributed instances
	locker Locker
	// monitor for reporting metrics
	monitor Monitor
	//
}

type jobIn struct {
	id                              uuid.UUID
	shouldSendOut                   bool
	triggerBeforeStartTime          bool
	triggerOnJobLimitedRunsComplete bool
	scheduleTime                    time.Time
	selectRun                       bool
	needRemoveJob                   bool
	// 下面两个只有在limitMode 下需要用到,保证正确的释放锁
	singletonMode  bool
	finalLimitMode LimitMode
}

type singletonRunner struct {
	in                chan jobIn
	rescheduleLimiter chan struct{}
}

type limitModeConfig struct {
	started           bool
	mode              LimitMode
	limit             uint
	rescheduleLimiter chan struct{}
	in                chan jobIn
	// singletonJobs is used to track singleton jobs that are running
	// in the limit mode runner. This is used to prevent the same job
	// from running multiple times across limit mode runners when both
	// a limit mode and singleton mode are enabled.
	singletonJobs   map[uuid.UUID]struct{}
	singletonJobsMu sync.Mutex

	jobLimitModeCanOverride bool
}

func (e *executor) start() {
	e.logger.Debug("gocron: executor started")

	// creating the executor's context here as the executor
	// is the only goroutine that should access this context
	// any other uses within the executor should create a context
	// using the executor context as parent.
	e.ctx, e.cancel = context.WithCancel(context.Background())

	// the standardJobsWg tracks
	standardJobsWg := &waitGroupWithMutex{}

	singletonJobsWg := &waitGroupWithMutex{}

	limitModeJobsWg := &waitGroupWithMutex{}

	// create a fresh map for tracking singleton runners
	e.singletonRunners = &sync.Map{}

	// start the for leap that is the executor
	// selecting on channels for work to do
	for {
		select {
		// job ids in are sent from 1 of 2 places:
		// 1. the scheduler sends directly when jobs
		//    are run immediately.
		// 2. sent from time.AfterFuncs in which job schedules
		// 	  are spun up by the scheduler
		case jIn := <-e.jobsIn:
			select {
			case <-e.stopCh:
				e.stop(standardJobsWg, singletonJobsWg, limitModeJobsWg)
				return
			default:
			}
			// this context is used to handle cancellation of the executor
			// on requests for a job to the scheduler via requestJobCtx
			ctx, cancel := context.WithCancel(e.ctx)

			if e.limitMode != nil && !e.limitMode.started {
				// check if we are already running the limit mode runners
				// if not, spin up the required number i.e. limit!
				e.limitMode.started = true
				for i := e.limitMode.limit; i > 0; i-- {
					limitModeJobsWg.Add(1)
					go e.limitModeRunner("limitMode-"+strconv.Itoa(int(i)), e.limitMode.in, limitModeJobsWg, e.limitMode.rescheduleLimiter)
				}
			}

			// spin off into a goroutine to unblock the executor and
			// allow for processing for more work
			go func() {
				// 处理 chan 出现阻塞,此时删除任务会关闭chan会导致出现异常 关闭的管道发送数据错误
				defer func() {
					if r := recover(); r != nil {
						fmt.Println("goCron: executor panicked", r)
						return
					}
				}()
				// make sure to cancel the above context per the docs
				// // Canceling this context releases resources associated with it, so code should
				// // call cancel as soon as the operations running in this Context complete.
				defer cancel()
				j := requestJobCtx(ctx, jIn.id, e.jobOutRequest)
				if j == nil {
					// safety check as it'd be strange bug if this occurred
					return
				}

				// check for limit mode - this spins up a separate runner which handles
				// limiting the total number of concurrently running jobs
				if e.limitMode != nil {
					finalLimitMode := e.limitMode.mode
					if j.singletonMode && e.limitMode.jobLimitModeCanOverride {
						finalLimitMode = j.singletonLimitMode
					}
					// limitMode 模式下 防止任务执行中更新job参数或者删除job无法正确释放锁
					jIn.singletonMode = j.singletonMode
					jIn.finalLimitMode = finalLimitMode
					if j.singletonMode {

						if finalLimitMode == LimitModeReschedule {
							// 如果rescheduleLimiter或者 singletonJobRunning 阻塞都应该跳过执行
							// rescheduleLimiter 满了代表所有的协程都在运行任务
							// singletonJobRunning 满了代表任务在执行中
							select {
							case e.limitMode.rescheduleLimiter <- struct{}{}:
								select {
								case j.singletonJobRunning <- struct{}{}:
									select {
									case e.limitMode.in <- jIn:
										e.sendOutForRescheduling(&jIn)
									case <-j.ctx.Done():
										return
									case <-e.ctx.Done():
										return
									}
								case <-j.ctx.Done():
									return
								case <-e.ctx.Done():
									return
								default:
									e.incrementJobCounter(*j, SingletonRescheduled)
									e.sendOutForReschedulingAndIgnoreThisExecution(&jIn)
								}
							case <-j.ctx.Done():
								return
							case <-e.ctx.Done():
								return
							default:
								e.incrementJobCounter(*j, SingletonRescheduled)
								e.sendOutForReschedulingAndIgnoreThisExecution(&jIn)
							}
						} else {
							select {
							case j.singletonJobRunning <- struct{}{}:
								select {
								case e.limitMode.in <- jIn:
									e.sendOutForRescheduling(&jIn)
								case <-j.ctx.Done():
									return
								case <-e.ctx.Done():
									return
								}
							case <-j.ctx.Done():
								return
							case <-e.ctx.Done():
								return
							}
						}

					} else {
						if finalLimitMode == LimitModeReschedule {
							select {
							case e.limitMode.rescheduleLimiter <- struct{}{}:
								select {
								case e.limitMode.in <- jIn:
									e.sendOutForRescheduling(&jIn)
								case <-j.ctx.Done():
									return
								case <-e.ctx.Done():
									return
								}
							case <-j.ctx.Done():
								return
							case <-e.ctx.Done():
								return
							default:
								e.incrementJobCounter(*j, Rescheduled)
								e.sendOutForReschedulingAndIgnoreThisExecution(&jIn)
							}
						} else {
							e.limitMode.in <- jIn
							e.sendOutForRescheduling(&jIn)
						}

					}

				} else {
					// no limit mode, so we're either running a regular job or
					// a job with a singleton mode
					//
					// get the job, so we can figure out what kind it is and how
					// to execute it

					if j.singletonMode {
						// for singleton mode, get the existing runner for the job
						// or spin up a new one
						runner := &singletonRunner{}
						runnerSrc, ok := e.singletonRunners.Load(jIn.id)
						if !ok {
							runner.in = make(chan jobIn, 1000)
							if j.singletonLimitMode == LimitModeReschedule {
								runner.rescheduleLimiter = make(chan struct{}, 1)
							}
							e.singletonRunners.Store(jIn.id, runner)
							singletonJobsWg.Add(1)
							go e.singletonModeRunner("singleton-"+jIn.id.String(), runner.in, singletonJobsWg, runner.rescheduleLimiter, j.ctx)
						} else {
							runner = runnerSrc.(*singletonRunner)
						}
						jIn.finalLimitMode = j.singletonLimitMode
						if j.singletonLimitMode == LimitModeReschedule {
							// reschedule mode uses the limiter channel to check
							// for a running job and reschedules if the channel is full.
							select {
							case runner.rescheduleLimiter <- struct{}{}:
								select {
								case runner.in <- jIn:
									e.sendOutForRescheduling(&jIn)
								case <-j.ctx.Done():
									return
								case <-e.ctx.Done():
									return
								}
							case <-j.ctx.Done():
								return
							case <-e.ctx.Done():
								return
							default:
								// runner is busy, reschedule the work for later
								// which means we just skip it here and do nothing
								e.incrementJobCounter(*j, SingletonRescheduled)
								e.sendOutForReschedulingAndIgnoreThisExecution(&jIn)
							}
						} else {
							// wait mode, fill up that queue (buffered channel, so it's ok)
							select {
							case runner.in <- jIn:
								e.sendOutForRescheduling(&jIn)
							case <-j.ctx.Done():
								return
							case <-e.ctx.Done():
								return
							}
						}
					} else {
						select {
						case <-e.stopCh:
							e.stop(standardJobsWg, singletonJobsWg, limitModeJobsWg)
							return
						default:
						}
						// we've gotten to the basic / standard jobs --
						// the ones without anything special that just want
						// to be run. Add to the WaitGroup so that
						// stopping or shutting down can wait for the jobs to
						// complete.
						standardJobsWg.Add(1)
						go func(j internalJob) {
							e.runJob(j, jIn)
							standardJobsWg.Done()
						}(*j)
					}
				}
			}()
		case <-e.stopCh:
			e.stop(standardJobsWg, singletonJobsWg, limitModeJobsWg)
			return
		}
	}
}

func (e *executor) sendOutForRescheduling(jIn *jobIn) {
	if jIn.shouldSendOut {
		select {
		case e.jobsOutForRescheduling <- *jIn:
		case <-e.ctx.Done():
			return
		}
	}
	// we need to set this to false now, because to handle
	// non-limit jobs, we send out from the e.runJob function
	// and in this case we don't want to send out twice.
	jIn.shouldSendOut = false
}

func (e *executor) sendOutForReschedulingAndIgnoreThisExecution(jIn *jobIn) {
	if jIn.shouldSendOut {
		select {
		case e.jobsOutForRescheduling <- *jIn:
			ctx, cancel := context.WithCancel(e.ctx)
			j := requestJobCtx(ctx, jIn.id, e.jobOutRequest)
			cancel()

			if jIn.triggerBeforeStartTime {
				_ = callJobFuncWithParams(j.beforeJobStartTime, j.id, j.name)
			}

			oTJ, isOk := j.jobSchedule.(oneTimeJob)
			if isOk {
				lenSortedTimes := len(oTJ.sortedTimes)
				if jIn.scheduleTime == oTJ.sortedTimes[lenSortedTimes-1] {
					jIn.needRemoveJob = true
					_ = callJobFuncWithParams(j.afterOneTimeJobAllRunComplete, j.id, j.name)
				}
			}

			if jIn.triggerOnJobLimitedRunsComplete {
				jIn.needRemoveJob = true
				_ = callJobFuncWithParams(j.onJobLimitedRunsComplete, j.id, j.name)
			}

			if j.stopTimeReached(e.clock.Now()) {
				j.hookOnce.Do(func() {
					jIn.needRemoveJob = true
					_ = callJobFuncWithParams(j.afterJobStopTime, j.id, j.name)
				})
			}

			select {
			case e.jobsOutCompleted <- *jIn:
			case <-e.ctx.Done():
			}
		case <-e.ctx.Done():
			return
		}
	}

	// we need to set this to false now, because to handle
	// non-limit jobs, we send out from the e.runJob function
	// and in this case we don't want to send out twice.
	jIn.shouldSendOut = false
}

func (e *executor) limitModeRunner(name string, in chan jobIn, wg *waitGroupWithMutex, rescheduleLimiter chan struct{}) {
	e.logger.Debug("gocron: limitModeRunner starting", "name", name)
	for {
		select {
		case jIn := <-in:
			select {
			case <-e.ctx.Done():
				e.logger.Debug("gocron: limitModeRunner shutting down", "name", name)
				wg.Done()
				return
			default:
			}

			ctx, cancel := context.WithCancel(e.ctx)
			j := requestJobCtx(ctx, jIn.id, e.jobOutRequest)
			cancel()
			if j != nil {
				jIn.shouldSendOut = false
				e.runJob(*j, jIn)
				if jIn.singletonMode {
					<-j.singletonJobRunning
				}
			}
			// remove the limiter block to allow another job to be scheduled
			if jIn.finalLimitMode == LimitModeReschedule {
				<-rescheduleLimiter
			}

		case <-e.ctx.Done():
			e.logger.Debug("limitModeRunner shutting down", "name", name)
			wg.Done()
			return
		}
	}
}

func (e *executor) singletonModeRunner(name string, in chan jobIn, wg *waitGroupWithMutex, rescheduleLimiter chan struct{}, jobCtx context.Context) {
	e.logger.Debug("gocron: singletonModeRunner starting", "name", name)
	for {
		select {
		case jIn := <-in:
			select {
			case <-e.ctx.Done():
				e.logger.Debug("gocron: singletonModeRunner shutting down", "name", name)
				wg.Done()
				return
			default:
			}

			ctx, cancel := context.WithCancel(e.ctx)
			j := requestJobCtx(ctx, jIn.id, e.jobOutRequest)
			cancel()
			if j != nil {
				// need to set shouldSendOut = false here, as there is a duplicative call to sendOutForRescheduling
				// inside the runJob function that needs to be skipped. sendOutForRescheduling is previously called
				// when the job is sent to the singleton mode runner.
				jIn.shouldSendOut = false
				e.runJob(*j, jIn)
			}

			// remove the limiter block to allow another job to be scheduled
			if jIn.finalLimitMode == LimitModeReschedule {
				<-rescheduleLimiter
			}
		case <-e.ctx.Done():
			e.logger.Debug("singletonModeRunner shutting down", "name", name)
			wg.Done()
			return
		case <-jobCtx.Done():
			e.logger.Debug("singletonModeRunner shutting down", "name", name)
			wg.Done()
			return

		}
	}
}

func (e *executor) runJob(j internalJob, jIn jobIn) {
	if j.ctx == nil {
		return
	}
	select {
	case <-e.ctx.Done():
		return
	case <-j.ctx.Done():
		return
	default:
	}

	if j.stopTimeReached(e.clock.Now()) {
		j.hookOnce.Do(func() {
			_ = callJobFuncWithParams(j.afterJobStopTime, j.id, j.name)
			jIn.needRemoveJob = true
			select {
			case e.jobsOutCompleted <- jIn:
			case <-e.ctx.Done():
			}
		})
		return
	}

	if e.elector != nil {
		if err := e.elector.IsLeader(j.ctx); err != nil {
			e.sendOutForRescheduling(&jIn)
			e.incrementJobCounter(j, Skip)
			return
		}
	} else if j.locker != nil {
		lock, err := j.locker.Lock(j.ctx, j.name)
		if err != nil {
			_ = callJobFuncWithParams(j.afterLockError, j.id, j.name, err)
			e.sendOutForRescheduling(&jIn)
			e.incrementJobCounter(j, Skip)
			return
		}
		defer func() { _ = lock.Unlock(j.ctx) }()
	} else if e.locker != nil {
		lock, err := e.locker.Lock(j.ctx, j.name)
		if err != nil {
			_ = callJobFuncWithParams(j.afterLockError, j.id, j.name, err)
			e.sendOutForRescheduling(&jIn)
			e.incrementJobCounter(j, Skip)
			return
		}
		defer func() { _ = lock.Unlock(j.ctx) }()
	}
	if jIn.triggerBeforeStartTime {
		_ = callJobFuncWithParams(j.beforeJobStartTime, j.id, j.name)
	}

	beforeJobReturnValue := callJobFuncHasReturnWithParams(j.beforeJobRuns, j.id, j.name)
	e.sendOutForRescheduling(&jIn)

	startTime := time.Now()
	err := e.callJobWithRecover(j)
	e.recordJobTiming(startTime, time.Now(), j)
	if err != nil {
		_ = callJobFuncWithParams(j.afterJobRunsWithError, j.id, j.name, err, beforeJobReturnValue)
		e.incrementJobCounter(j, Fail)
	} else {
		_ = callJobFuncWithParams(j.afterJobRuns, j.id, j.name, beforeJobReturnValue)
		e.incrementJobCounter(j, Success)
	}
	oTJ, isOk := j.jobSchedule.(oneTimeJob)
	if isOk {
		lenSortedTimes := len(oTJ.sortedTimes)
		if lenSortedTimes == 0 || jIn.scheduleTime == oTJ.sortedTimes[lenSortedTimes-1] {
			jIn.needRemoveJob = true
			_ = callJobFuncWithParams(j.afterOneTimeJobAllRunComplete, j.id, j.name)
		}
	}

	if jIn.triggerOnJobLimitedRunsComplete {
		jIn.needRemoveJob = true
		_ = callJobFuncWithParams(j.onJobLimitedRunsComplete, j.id, j.name)
	}

	if j.stopTimeReached(e.clock.Now()) {
		j.hookOnce.Do(func() {
			jIn.needRemoveJob = true
			_ = callJobFuncWithParams(j.afterJobStopTime, j.id, j.name)
		})
	}

	select {
	case e.jobsOutCompleted <- jIn:
	case <-e.ctx.Done():
	}

}

func (e *executor) callJobWithRecover(j internalJob) (err error) {
	defer func() {
		if recoverData := recover(); recoverData != nil {
			_ = callJobFuncWithParams(j.afterJobRunsWithPanic, j.id, j.name, recoverData)

			// if panic is occurred, we should return an error
			err = fmt.Errorf("%w from %v", ErrPanicRecovered, recoverData)
		}
	}()

	return callJobFuncWithParams(j.function, j.parameters...)
}

func (e *executor) recordJobTiming(start time.Time, end time.Time, j internalJob) {
	if e.monitor != nil {
		e.monitor.RecordJobTiming(start, end, j.id, j.name, j.tags)
	}
}

func (e *executor) incrementJobCounter(j internalJob, status JobStatus) {
	if e.monitor != nil {
		e.monitor.IncrementJob(j.id, j.name, j.tags, status)
	}
}

func (e *executor) stop(standardJobsWg, singletonJobsWg, limitModeJobsWg *waitGroupWithMutex) {
	e.logger.Debug("gocron: stopping executor")
	// we've been asked to stop. This is either because the scheduler has been told
	// to stop all jobs or the scheduler has been asked to completely shutdown.
	//
	// cancel tells all the functions to stop their work and send in a done response
	e.cancel()

	// the wait for job channels are used to report back whether we successfully waited
	// for all jobs to complete or if we hit the configured timeout.
	waitForJobs := make(chan struct{}, 1)
	waitForSingletons := make(chan struct{}, 1)
	waitForLimitMode := make(chan struct{}, 1)

	// the waiter context is used to cancel the functions waiting on jobs.
	// this is done to avoid goroutine leaks.
	waiterCtx, waiterCancel := context.WithCancel(context.Background())

	// wait for standard jobs to complete
	go func() {
		e.logger.Debug("gocron: waiting for standard jobs to complete")
		go func() {
			// this is done in a separate goroutine, so we aren't
			// blocked by the WaitGroup's Wait call in the event
			// that the waiter context is cancelled.
			// This particular goroutine could leak in the event that
			// some long-running standard job doesn't complete.
			standardJobsWg.Wait()
			e.logger.Debug("gocron: standard jobs completed")
			waitForJobs <- struct{}{}
		}()
		<-waiterCtx.Done()
	}()

	// wait for per job singleton limit mode runner jobs to complete
	go func() {
		e.logger.Debug("gocron: waiting for singleton jobs to complete")
		go func() {
			singletonJobsWg.Wait()
			e.logger.Debug("gocron: singleton jobs completed")
			waitForSingletons <- struct{}{}
		}()
		<-waiterCtx.Done()
	}()

	// wait for limit mode runners to complete
	go func() {
		e.logger.Debug("gocron: waiting for limit mode jobs to complete")
		go func() {
			limitModeJobsWg.Wait()
			e.logger.Debug("gocron: limitMode jobs completed")
			waitForLimitMode <- struct{}{}
		}()
		<-waiterCtx.Done()
	}()

	// now either wait for all the jobs to complete,
	// or hit the timeout.
	var count int
	timeout := time.Now().Add(e.stopTimeout)
	for time.Now().Before(timeout) && count < 3 {
		select {
		case <-waitForJobs:
			count++
		case <-waitForSingletons:
			count++
		case <-waitForLimitMode:
			count++
		default:
		}
	}
	if count < 3 {
		e.done <- ErrStopJobsTimedOut
		e.logger.Debug("gocron: executor stopped - timed out")
	} else {
		e.done <- nil
		e.logger.Debug("gocron: executor stopped")
	}
	waiterCancel()

	if e.limitMode != nil {
		e.limitMode.started = false
	}
}
