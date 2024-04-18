package test

import (
	"fmt"
	"github.com/acexy/golang-toolkit/logger"
	"github.com/golang-acexy/starter-cron/cronmodule"
	"github.com/golang-acexy/starter-parent/parentmodule/declaration"
	"github.com/sirupsen/logrus"
	"sync/atomic"
	"testing"
	"time"
)

var m declaration.Module
var count1 int32
var count2 int32

func init() {
	l := &logger.LogrusConfig{}
	l.EnableConsole(logrus.TraceLevel, false)
	m = declaration.Module{
		ModuleLoaders: []declaration.ModuleLoader{&cronmodule.CronModule{
			EnableLogger: false,
		}},
	}
	err := m.Load()
	if err != nil {
		println(err)
		return
	}
}
func TestLoadAndUnLoad(t *testing.T) {
	cronmodule.Start()
	fmt.Println(m.UnloadByConfig())
}

func TestAddSimpleJob(t *testing.T) {
	cronmodule.AddSimpleJob("@every 1s", func() {
		fmt.Println(time.Now().Format("15:04:05"), "执行中")
		time.Sleep(time.Second * 5)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now().Format("15:04:05"), "执行完成")
	})
	time.Sleep(time.Second * 30)
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestAddSimpleSingletonJob(t *testing.T) {
	cronmodule.AddSimpleSingletonJob("@every 1s", func() {
		fmt.Println(time.Now().Format("15:04:05"), "执行中")
		time.Sleep(time.Second * 5)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now().Format("15:04:05"), "执行完成")
	})
	time.Sleep(time.Second * 30)
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestJobFlushSpec(t *testing.T) {
	spec1 := "@every 1s"
	task1 := cronmodule.NewJob("task1", &spec1, false, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
		time.Sleep(time.Second * 2)
	}, true)

	_ = task1.Register()
	time.Sleep(time.Second * 5)

	_ = task1.FlushSpec("@every 2s")
	time.Sleep(time.Second * 10)

	_ = task1.FlushSpec("@every 1s")
	time.Sleep(time.Second * 10)
}

func TestJobsFlushSpec(t *testing.T) {

	spec1 := "@every 1s"
	spec2 := "@every 1s"

	task1 := cronmodule.NewJob("task1", &spec1, false, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
	})

	task2 := cronmodule.NewJob("task2", &spec2, false, func() {
		fmt.Println("task2 invoke", time.Now().Format("15:04:05"))
		time.Sleep(2 * time.Second)
	}, true)

	_ = task1.Register()
	_ = task2.Register()
	time.Sleep(time.Second * 10)

	_ = task1.FlushSpec("@every 1s")
	_ = task2.FlushSpec("@every 1s")

	time.Sleep(time.Second * 10)
}

func TestJobAutoFlushSpec(t *testing.T) {
	spec1 := "@every 1s"
	task1 := cronmodule.NewJob("task1", &spec1, true, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
	}, false)
	_ = task1.Register()

	go func() {
		time.Sleep(time.Second * 5)
		spec1 = "@every 2s"
		time.Sleep(time.Second * 5)
		spec1 = "@every 3s"
		time.Sleep(time.Second * 5)
		spec1 = "@every 4s"
	}()

	time.Sleep(time.Second * 30)
}

func TestUnload(t *testing.T) {
	m.Unload(10)
}
