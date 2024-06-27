# starter-cron

基于`github.com/robfig/cron`封装的定时任务调度组件

---

#### 功能说明

- 新增支持同一任务执行中忽略下次调度设置
```go
func TestAddSimpleSingletonJob(t *testing.T) {
	cronstrater.AddSimpleSingletonJob("@every 1s", func() {
		fmt.Println(time.Now().Format("15:04:05"), "执行中")
		time.Sleep(time.Second * 5)
		atomic.AddInt32(&count2, 1)
		fmt.Println(time.Now().Format("15:04:05"), "执行完成")
	})
	time.Sleep(time.Second * 30)
	fmt.Println("init func invoke count", count1, "AddJob invoke count", count2)
}
```
- 支持执行中的定时任务动态刷新执行计划(下次执行中触发)
```go
func TestJobsFlushSpec(t *testing.T) {

	spec1 := "@every 1s"
	spec2 := "@every 1s"

	task1 := cronstrater.NewJob("task1", &spec1, false, func() {
		fmt.Println("task1 invoke", time.Now().Format("15:04:05"))
	})

	task2 := cronstrater.NewJob("task2", &spec2, false, func() {
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
	_ = cronstrater.NewJobAndRegister("task1", &spec1, true, func() {
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
```