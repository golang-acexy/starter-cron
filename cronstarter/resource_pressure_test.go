package cronstarter

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"
)

func TestCronResourcePressure(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()
	runtime.GC()
	beforeG := runtime.NumGoroutine()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	var jobs []weak.Pointer[jobFunc]
	var payloads []weak.Pointer[[4096]byte]
	start := time.Now()
	for round := 0; round < 20; round++ {
		starter := &CronStarter{}
		if _, err := starter.Start(); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 500; i++ {
			func() {
				spec := "@every 1h"
				payload := new([4096]byte)
				payload[0] = byte(i)
				j := NewJob("pressure", &spec, false, func() { runtime.KeepAlive(payload) })
				jobs = append(jobs, weak.Make(j))
				payloads = append(payloads, weak.Make(payload))
				if err := j.Register(); err != nil {
					t.Fatal(err)
				}
				if err := j.FlushSpec("@every 2h"); err != nil {
					t.Fatal(err)
				}
				if err := j.Remove(); err != nil {
					t.Fatal(err)
				}
			}()
		}
		if entries := RawCron().Entries(); len(entries) != 0 {
			t.Fatalf("remaining entries: %d", len(entries))
		}
		if _, _, err := starter.Stop(time.Second); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	deadline := time.Now().Add(3 * time.Second)
	for {
		runtime.GC()
		live := 0
		for _, p := range jobs {
			if p.Value() != nil {
				live++
			}
		}
		for _, p := range payloads {
			if p.Value() != nil {
				live++
			}
		}
		if live == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d job/payload objects retained", live)
		}
		time.Sleep(10 * time.Millisecond)
	}
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	jobListLock.Lock()
	count := len(jobList)
	jobListLock.Unlock()
	if count != 0 || RawCron() != nil || cronStopping.Load() {
		t.Fatal("runtime did not drain")
	}
	t.Logf("20 lifetimes, 10000 register/replace/remove cycles; live jobs/payloads=0; elapsed=%s goroutines=%d->%d heap=%d->%d", elapsed, beforeG, runtime.NumGoroutine(), before.HeapAlloc, after.HeapAlloc)
}

func TestCronRealDispatchAndStopTimeout(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()
	starter := &CronStarter{}
	if _, err := starter.Start(); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	id, err := AddSimpleSingletonJob("@every 1s", func() { once.Do(func() { close(entered) }); <-release })
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("real scheduler did not dispatch")
	}
	RawCron().Remove(id)
	graceful, stopped, err := starter.Stop(time.Millisecond)
	if graceful || !stopped || !errors.Is(err, ErrCronStopTimeout) {
		close(release)
		t.Fatalf("unexpected timeout: %v %v %v", graceful, stopped, err)
	}
	if _, err := starter.Start(); !errors.Is(err, ErrCronStarterAlreadyStarted) {
		close(release)
		t.Fatal("started while a callback was still running")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for cronStopping.Load() {
		if time.Now().After(deadline) {
			t.Fatal("draining did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := starter.Start(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := starter.Stop(time.Second); err != nil {
		t.Fatal(err)
	}
}

func startCronRegression(t *testing.T) {
	t.Helper()

	resetCronStarterForTest()
	t.Cleanup(resetCronStarterForTest)
	starter := &CronStarter{Config: CronConfig{ManualStart: true}}
	if _, err := starter.Start(); err != nil {
		t.Fatal(err)
	}
}

func TestCronRegressionDynamicCallbackRace(t *testing.T) {
	startCronRegression(t)
	if err := NewJobAndRegisterWithNewSpec("dynamic", "@every 1h", func() string { return "@every 1h" }, true); err != nil {
		t.Fatal(err)
	}
	wrapper := RawCron().Entries()[0].WrappedJob
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				wrapper.Run()
			}
		}()
	}
	wg.Wait()
}

func TestCronRegressionOldHandleOwnership(t *testing.T) {
	startCronRegression(t)
	spec := "@every 1h"
	old := NewJob("same", &spec, false, func() {})
	if err := old.Register(); err != nil {
		t.Fatal(err)
	}
	if err := old.Remove(); err != nil {
		t.Fatal(err)
	}
	current := NewJob("same", &spec, false, func() {})
	if err := current.Register(); err != nil {
		t.Fatal(err)
	}
	err := old.Remove()
	if err == nil || len(RawCron().Entries()) != 1 {
		t.Fatal("old handle removed the new owner's job")
	}
}

func TestCronRegressionInvalidReplacement(t *testing.T) {
	startCronRegression(t)
	spec := "@every 1h"
	j := NewJob("replace", &spec, false, func() {})
	if err := j.Register(); err != nil {
		t.Fatal(err)
	}
	if err := j.FlushSpec("invalid"); err == nil {
		t.Fatal("expected parser error")
	}
	if len(RawCron().Entries()) != 1 {
		t.Fatal("invalid replacement removed the previous valid schedule")
	}
}

func TestCronRegressionSingletonAcrossReplacement(t *testing.T) {
	startCronRegression(t)
	spec := "@every 1h"
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var running atomic.Int32
	var wg sync.WaitGroup
	defer func() { close(release); wg.Wait() }()
	j := NewJob("singleton", &spec, false, func() { running.Add(1); entered <- struct{}{}; <-release })
	if err := j.Register(); err != nil {
		t.Fatal(err)
	}
	first := RawCron().Entries()[0].WrappedJob
	wg.Add(1)
	go func() { defer wg.Done(); first.Run() }()
	<-entered
	if err := j.FlushSpec("@every 2h"); err != nil {
		t.Fatal(err)
	}
	second := RawCron().Entries()[0].WrappedJob
	wg.Add(1)
	go func() { defer wg.Done(); second.Run() }()
	select {
	case <-entered:
		t.Fatalf("singleton callbacks overlapped across replacement: %d", running.Load())
	case <-time.After(50 * time.Millisecond):
	}
}

func BenchmarkCronRegisterReplaceRemove(b *testing.B) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()
	starter := &CronStarter{Config: CronConfig{ManualStart: true}}
	if _, err := starter.Start(); err != nil {
		b.Fatal(err)
	}
	spec := "@every 1h"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		j := NewJob("bench", &spec, false, func() {})
		if err := j.Register(); err != nil {
			b.Fatal(err)
		}
		if err := j.FlushSpec("@every 2h"); err != nil {
			b.Fatal(err)
		}
		if err := j.Remove(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCronCallbackWrapper(b *testing.B) {
	for _, named := range []bool{false, true} {
		b.Run(fmt.Sprint(named), func(b *testing.B) {
			j := &job{cmd: func() {}}
			if named {
				j.jobFunc = &jobFunc{}
			}
			b.ReportAllocs()
			for b.Loop() {
				j.Run()
			}
		})
	}
}
