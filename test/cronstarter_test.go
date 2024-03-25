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
			EnableLogger: true,
			Func: []cronmodule.JobFunc{{
				Spec: "@every 1s",
				Cmd: func() {
					fmt.Println(time.Now(), "init func 开始执行.")
					time.Sleep(time.Second * 2)
					atomic.AddInt32(&count1, 1)
					fmt.Println(time.Now(), "init func 执行完毕.")
				},
			}},
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

func TestAddJob(t *testing.T) {
	cronmodule.AddJob("@every 1s", func() {
		fmt.Println(time.Now(), "执行中")
		time.Sleep(time.Second * 2)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now(), "执行完成")

	})
	time.Sleep(time.Second * 30)
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestAddSingletonJob(t *testing.T) {
	cronmodule.AddSingletonJob("@every 1s", func() {
		fmt.Println(time.Now(), "执行中")
		time.Sleep(time.Second * 2)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now(), "执行完成")

	})
	time.Sleep(time.Second * 30)
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestUnload(t *testing.T) {
	m.Unload(10)
}
