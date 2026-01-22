package test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/acexy/golang-toolkit/logger"
	"github.com/acexy/golang-toolkit/sys"
	"github.com/acexy/golang-toolkit/util/json"
	"github.com/golang-acexy/starter-cron/cronstarter"
	"github.com/golang-acexy/starter-parent/parent"
)

var starterLoader *parent.StarterLoader
var count1 int32
var count2 int32

func init() {
	logger.EnableConsole(logger.TraceLevel, false)
	starterLoader = parent.NewStarterLoader([]parent.Starter{
		&cronstarter.CronStarter{
			Config: cronstarter.CronConfig{EnableLogger: true},
		},
	})
	err := starterLoader.Start()
	if err != nil {
		println(err)
		return
	}
}
func TestLoadAndUnLoad(t *testing.T) {
	cronstarter.Start() // 忽略重复启动
	stopResult, err := starterLoader.Stop(time.Second * 10)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}
	fmt.Println(json.ToStringFormat(stopResult))
}

func TestAddSimpleJob(t *testing.T) {
	cronstarter.AddSimpleJob("@every 1s", func() {
		fmt.Println(time.Now().Format("15:04:05"), "执行中")
		time.Sleep(time.Second * 5)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now().Format("15:04:05"), "执行完成")
		var i int
		fmt.Println(1 / i)
	})
	sys.ShutdownHolding()
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestAddSimpleSingletonJob(t *testing.T) {
	cronstarter.AddSimpleSingletonJob("@every 1s", func() {
		fmt.Println(time.Now().Format("15:04:05"), "执行中")
		time.Sleep(time.Second * 5)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now().Format("15:04:05"), "执行完成")
	})
	sys.ShutdownHolding()
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}

func TestJobFlushSpec(t *testing.T) {
	spec1 := "@every 1s"
	task1 := cronstarter.NewJob("task1", &spec1, false, func() {
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

	task1 := cronstarter.NewJob("task1", &spec1, false, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
	})

	task2 := cronstarter.NewJob("task2", &spec2, false, func() {
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
	_ = cronstarter.NewJobAndRegister("task1", &spec1, true, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
	}, false)

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
