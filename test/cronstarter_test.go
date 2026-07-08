package test

import (
	"testing"
	"time"

	"github.com/golang-acexy/starter-cron/cronstarter"
)

func TestCronStarterLifecycle(t *testing.T) {
	starter := &cronstarter.CronStarter{
		Config: cronstarter.CronConfig{ManualStart: true},
	}
	if _, err := starter.Start(); err != nil {
		t.Fatalf("start cron starter failed: %v", err)
	}
	if cronstarter.RawCron() == nil {
		t.Fatal("raw cron should not be nil after starter start")
	}
	if err := cronstarter.Start(); err != nil {
		t.Fatalf("manual start failed: %v", err)
	}
	gracefully, stopped, err := starter.Stop(time.Second)
	if err != nil {
		t.Fatalf("stop cron starter failed: %v", err)
	}
	if !gracefully || !stopped {
		t.Fatalf("unexpected stop result, gracefully=%v stopped=%v", gracefully, stopped)
	}
}
