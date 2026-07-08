package cronstarter

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetCronStarterForTest() {
	if instance := RawCron(); instance != nil {
		instance.Stop()
	}
	cronInstanceLock.Lock()
	cronInstance = nil
	cronInstanceLock.Unlock()
	jobListLock.Lock()
	jobList = make(map[string]*jobInfo)
	jobListLock.Unlock()
}

func TestRegisterBeforeStartReturnsError(t *testing.T) {
	resetCronStarterForTest()

	spec := "@every 1s"
	err := NewJob("not-started", &spec, false, func() {}).Register()
	if !errors.Is(err, ErrCronStarterNotStarted) {
		t.Fatalf("expected ErrCronStarterNotStarted, got %v", err)
	}
}

func TestJobRegisterFlushAndRemove(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()

	starter := &CronStarter{Config: CronConfig{ManualStart: true}}
	if _, err := starter.Start(); err != nil {
		t.Fatalf("start cron starter failed: %v", err)
	}

	spec := "@every 1h"
	task := NewJob("task", &spec, false, func() {})
	if err := task.Register(); err != nil {
		t.Fatalf("register job failed: %v", err)
	}
	if err := task.Register(); !errors.Is(err, ErrJobAlreadyExists) {
		t.Fatalf("expected ErrJobAlreadyExists, got %v", err)
	}
	if err := task.FlushSpec("@every 2h"); err != nil {
		t.Fatalf("flush job spec failed: %v", err)
	}
	if err := task.Remove(); err != nil {
		t.Fatalf("remove job failed: %v", err)
	}
	if err := task.Remove(); !errors.Is(err, ErrJobNotExists) {
		t.Fatalf("expected ErrJobNotExists, got %v", err)
	}
}

func TestSimpleJobRunsAndRecoversPanic(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()

	starter := &CronStarter{}
	if _, err := starter.Start(); err != nil {
		t.Fatalf("start cron starter failed: %v", err)
	}

	done := make(chan struct{}, 1)
	_, err := AddSimpleJob("@every 10ms", func() {
		done <- struct{}{}
		panic("expected panic")
	})
	if err != nil {
		t.Fatalf("add simple job failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("simple job was not invoked")
	}
}

func TestSingletonJobSkipsConcurrentRun(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var closeStarted sync.Once
	var runCount int32

	task := &job{cmd: func() {
		atomic.AddInt32(&runCount, 1)
		closeStarted.Do(func() {
			close(started)
		})
		<-release
	}}

	go task.Run()
	<-started
	task.Run()

	if atomic.LoadInt32(&runCount) != 1 {
		t.Fatalf("singleton job should skip concurrent run, got %d", runCount)
	}
	close(release)
}

func TestNewJobAndRegisterWithNewSpec(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()

	starter := &CronStarter{Config: CronConfig{ManualStart: true}}
	if _, err := starter.Start(); err != nil {
		t.Fatalf("start cron starter failed: %v", err)
	}

	err := NewJobAndRegisterWithNewSpec("dynamic-spec", "@every 1h", func() string {
		return "@every 2h"
	})
	if err != nil {
		t.Fatalf("register dynamic spec job failed: %v", err)
	}
}
