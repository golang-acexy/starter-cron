package cronstarter

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestScheduleReplacementPreservesSettingsOnError(t *testing.T) {
	startCronRegression(t)
	spec := "@every 1h"
	j := NewJob("preserve", &spec, true, func() {})
	if err := j.Register(); err != nil {
		t.Fatal(err)
	}
	original := RawCron().Entries()[0]
	if err := j.FlushSpec("invalid"); err == nil {
		t.Fatal("invalid spec accepted")
	}
	if got := RawCron().Entries(); len(got) != 1 || got[0].ID != original.ID {
		t.Fatal("failed update changed entry")
	}
	if j.spec != &spec || !j.autoReloadSpec {
		t.Fatal("failed update changed borrowed spec or automatic reload")
	}
	spec = "@every 2h"
	original.WrappedJob.Run()
	if got := RawCron().Entries(); len(got) != 1 || got[0].ID == original.ID {
		t.Fatal("borrowed expression did not reload")
	}
	if j.spec != &spec {
		t.Fatal("automatic reload detached borrowed pointer")
	}
}

func TestOldHandleCannotReplaceNewOwner(t *testing.T) {
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
	id := RawCron().Entries()[0].ID
	if err := old.FlushSpec("@every 2h"); !errors.Is(err, ErrJobNotExists) {
		t.Fatalf("old handle update: %v", err)
	}
	if got := RawCron().Entries(); len(got) != 1 || got[0].ID != id {
		t.Fatal("old handle replaced new owner's schedule")
	}
}

func TestAutomaticResultCannotOverwriteNewRegistration(t *testing.T) {
	for _, mode := range []string{"automatic replacement", "manual replacement", "same handle registration"} {
		t.Run(mode, func(t *testing.T) {
			startCronRegression(t)
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			spec := "@every 1h"
			j := NewJob("late", &spec, true, nil, true)
			j.nextSpec = func() string { close(entered); <-release; return "@every 2h" }
			if err := j.Register(); err != nil {
				t.Fatal(err)
			}
			first := RawCron().Entries()[0].WrappedJob
			go func() { defer close(done); first.Run() }()
			<-entered
			switch mode {
			case "automatic replacement":
				j.Lock()
				err := j.replaceLocked("@every 3h", first.(*job))
				j.Unlock()
				if err != nil {
					close(release)
					<-done
					t.Fatal(err)
				}
			case "manual replacement":
				if err := j.FlushSpec("@every 3h"); err != nil {
					close(release)
					<-done
					t.Fatal(err)
				}
			case "same handle registration":
				if err := j.Remove(); err != nil {
					close(release)
					<-done
					t.Fatal(err)
				}
				if err := j.Register(); err != nil {
					close(release)
					<-done
					t.Fatal(err)
				}
			}
			current := RawCron().Entries()[0].ID
			close(release)
			<-done
			if got := RawCron().Entries(); len(got) != 1 || got[0].ID != current {
				t.Fatal("late result changed replacement registration")
			}
		})
	}
}

func TestAutomaticAndManualUpdatesConcurrent(t *testing.T) {
	startCronRegression(t)
	if err := NewJobAndRegisterWithNewSpec("concurrent", "@every 1h", func() string { return "@every 2h" }, true); err != nil {
		t.Fatal(err)
	}
	j := jobList["concurrent"].jobFunc
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 100; k++ {
				entries := RawCron().Entries()
				if len(entries) > 0 {
					entries[0].WrappedJob.Run()
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		if err := j.FlushSpec(fmt.Sprintf("@every %dh", i%3+1)); err != nil {
			t.Error(err)
		}
	}
	wg.Wait()
	if len(RawCron().Entries()) != 1 {
		t.Fatal("concurrent updates lost or duplicated the entry")
	}
}

func TestRegisterAndManualStartAgainstStop(t *testing.T) {
	resetCronStarterForTest()
	defer resetCronStarterForTest()
	for round := 0; round < 30; round++ {
		starter := &CronStarter{Config: CronConfig{ManualStart: true}}
		if _, err := starter.Start(); err != nil {
			t.Fatal(err)
		}
		begin := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-begin
				spec := "@every 1h"
				err := NewJob(fmt.Sprint(i), &spec, false, func() {}).Register()
				if err != nil && !errors.Is(err, ErrCronStarterNotStarted) {
					t.Error(err)
				}
				if err := Start(); err != nil && !errors.Is(err, ErrCronStarterNotStarted) {
					t.Error(err)
				}
			}(i)
		}
		close(begin)
		if _, _, err := starter.Stop(time.Second); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		jobListLock.Lock()
		count := len(jobList)
		jobListLock.Unlock()
		if count != 0 || RawCron() != nil {
			t.Fatal("registration survived runtime withdrawal")
		}
	}
}

func TestConcurrentAutomaticReplacementKeepsOneEntry(t *testing.T) {
	startCronRegression(t)
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	if err := NewJobAndRegisterWithNewSpec("automatic", "@every 1h", func() string {
		entered <- struct{}{}
		<-release
		return "@every 2h"
	}, true); err != nil {
		t.Fatal(err)
	}
	original := RawCron().Entries()[0]
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); original.WrappedJob.Run() }()
	}
	for i := 0; i < 16; i++ {
		<-entered
	}
	close(release)
	wg.Wait()
	entries := RawCron().Entries()
	if len(entries) != 1 || entries[0].ID != original.ID+1 {
		t.Fatal("concurrent callback results replaced more than once or lost the entry")
	}
}

func TestCallbackReturnedInvalidExpressionPreservesEntry(t *testing.T) {
	startCronRegression(t)
	if err := NewJobAndRegisterWithNewSpec("invalid", "@every 1h", func() string { return "invalid" }); err != nil {
		t.Fatal(err)
	}
	original := RawCron().Entries()[0]
	original.WrappedJob.Run()
	entries := RawCron().Entries()
	if len(entries) != 1 || entries[0].ID != original.ID {
		t.Fatal("invalid automatic update removed the valid schedule")
	}
}
