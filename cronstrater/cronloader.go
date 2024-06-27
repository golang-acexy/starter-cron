package cronstrater

import (
	"errors"
	"github.com/acexy/golang-toolkit/logger"
	"github.com/golang-acexy/starter-parent/parent"
	"github.com/robfig/cron/v3"
	"sync"
	"time"
)

var cronInstance *cron.Cron

var jobList = make(map[string]*jobInfo)

type jobInfo struct {
	jobId   *cron.EntryID
	jobFunc *jobFunc
}

type job struct {
	m sync.Mutex

	originSpec string
	jobFunc    *jobFunc

	cmd func()
}

func (j *job) Run() {
	if j.jobFunc == nil {
		var flag = j.m.TryLock()
		if flag {
			defer j.m.Unlock()
		} else {
			return
		}
		j.cmd()
	} else {
		if !j.jobFunc.multiRun {
			var flag = j.m.TryLock()
			if flag {
				defer j.m.Unlock()
			} else {
				return
			}
		}
		j.cmd()
		if j.jobFunc.autoReloadSpec {
			if j.originSpec != *j.jobFunc.spec {
				go j.flushSpec()
			}
		}
	}

}

func (j *job) flushSpec() {
	err := j.jobFunc.Remove()
	if err != nil {
		logger.Logrus().WithError(err).Errorln("auto flush spec: remove job error")
		return
	}
	j.originSpec = *j.jobFunc.spec
	err = j.jobFunc.Register()
	if err != nil {
		logger.Logrus().WithError(err).Errorln("auto flush spec: register job error")
		return
	}
}

type jobFunc struct {
	sync.Mutex

	// 唯一任务名称
	jobName string

	// 是否允许同一个任务在上一个调度未完成时继续被调度执行
	multiRun bool

	// 执行表达式
	spec *string

	// 任务函数
	cmd func()

	// 如果启用，则每次任务函数完成后将自动检查任务表达式是否变化，如果变化则自动重新加载规则
	autoReloadSpec bool
}

type CronStarter struct {
	// 启动详细日志
	EnableLogger bool

	// 手动启动定时任务
	// 如果手动启动需要手动调用cronstrater.Start()方法启动整个任务执行器
	ManualStart bool
}

func (c *CronStarter) Setting() *parent.Setting {
	return parent.NewSetting("Cron-Starter", 10, true, time.Second*20, nil)
}

func (c *CronStarter) Start() (interface{}, error) {
	opts := make([]cron.Option, 0)
	if c.EnableLogger {
		opts = append(opts, cron.WithLogger(ll))
	}
	cronInstance = cron.New(opts...)
	if !c.ManualStart {
		cronInstance.Start()
	}
	return cronInstance, nil
}

func (c *CronStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	ctx := cronInstance.Stop()
	select {
	case <-ctx.Done():
		logger.Logrus().Traceln("")
		return true, true, nil
	case <-time.After(maxWaitTime):
		cronInstance.Start()
		return false, true, errors.New("waiting for cron starter shutdown timeout")
	}
}

// Start 启动已注册任务 如果CronModule.ManualStart = true时一定需要手动开启
func Start() {
	cronInstance.Start()
}

// NewJob 初始化一个Job配置
func NewJob(jobName string, spec *string, autoReloadSpec bool, cmd func(), multiRun ...bool) *jobFunc {
	j := &jobFunc{
		jobName:        jobName,
		spec:           spec,
		cmd:            cmd,
		autoReloadSpec: autoReloadSpec,
	}
	if len(multiRun) > 0 && multiRun[0] {
		j.multiRun = true
	}
	return j
}

// NewJobAndRegister 初始化一个Job配置 并注册
func NewJobAndRegister(jobName string, spec *string, autoReloadSpec bool, cmd func(), multiRun ...bool) error {
	return NewJob(jobName, spec, autoReloadSpec, cmd, multiRun...).Register()
}

// Register 注册该Job
func (j *jobFunc) Register() error {
	defer j.Unlock()
	j.Lock()
	_, flag := jobList[j.jobName]
	if flag {
		return errors.New("the job already exists : " + j.jobName)
	}
	id, err := cronInstance.AddJob(*j.spec, &job{
		cmd:        j.cmd,
		originSpec: *j.spec,
		jobFunc:    j,
	})
	if err != nil {
		return err
	}
	jobList[j.jobName] = &jobInfo{
		jobId:   &id,
		jobFunc: j,
	}
	return nil
}

// FlushSpec 更改Job规则 该操作将自动关闭 autoReloadSpec
func (j *jobFunc) FlushSpec(spec string) error {
	j.Lock()
	v, flag := jobList[j.jobName]
	if !flag {
		return errors.New("the job not exists : " + j.jobName)
	}
	j.Unlock()
	j.spec = &spec
	j.autoReloadSpec = false
	cronInstance.Remove(*v.jobId)
	delete(jobList, j.jobName)
	return j.Register()
}

// Remove 移除任务
func (j *jobFunc) Remove() error {
	defer j.Unlock()
	j.Lock()
	v, flag := jobList[j.jobName]
	if !flag {
		return errors.New("the job not exists : " + j.jobName)
	}
	cronInstance.Remove(*v.jobId)
	delete(jobList, j.jobName)
	return nil
}

// AddSimpleJob 添加简单任务
func AddSimpleJob(spec string, cmd func()) (cron.EntryID, error) {
	return cronInstance.AddFunc(spec, cmd)
}

// AddSimpleSingletonJob 添加简单单例任务 该任务将忽略正在运行的任务的调度
func AddSimpleSingletonJob(spec string, cmd func()) (cron.EntryID, error) {
	return cronInstance.AddJob(spec, &job{cmd: cmd})
}

// RawCron 获取原始的cron实例
func RawCron() *cron.Cron {
	return cronInstance
}
