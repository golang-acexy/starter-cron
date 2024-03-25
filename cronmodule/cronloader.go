package cronmodule

import (
	"errors"
	"github.com/acexy/golang-toolkit/logger"
	"github.com/golang-acexy/starter-parent/parentmodule/declaration"
	"github.com/robfig/cron/v3"
	"sync"
	"time"
)

var instance *cron.Cron

type singleJob struct {
	m   sync.Mutex
	cmd func()
}

func (s *singleJob) Run() {
	s.m.Lock()
	defer s.m.Unlock()
	s.cmd()
}

type JobFunc struct {

	// 是否支持运行中的任务继续被调度运行
	MultiRun bool
	// 执行表达式
	Spec string
	// 任务
	Cmd func()
}

type CronModule struct {
	// 启动日志
	EnableLogger bool
	// 手动启动定时任务
	ManualStart bool
	// 初始化的任务
	Func []JobFunc
}

func (c *CronModule) ModuleConfig() *declaration.ModuleConfig {
	return &declaration.ModuleConfig{
		ModuleName:               "Cron",
		UnregisterPriority:       10,
		UnregisterAllowAsync:     true,
		UnregisterMaxWaitSeconds: 60,
	}
}

func (c *CronModule) Register() (interface{}, error) {
	opts := make([]cron.Option, 0)
	if c.EnableLogger {
		opts = append(opts, cron.WithLogger(ll))
	}
	instance = cron.New(opts...)
	if len(c.Func) > 0 {
		for _, f := range c.Func {
			var funcId cron.EntryID
			var err error
			if f.MultiRun {
				funcId, err = instance.AddFunc(f.Spec, f.Cmd)
			} else {
				funcId, err = instance.AddJob(f.Spec, &singleJob{cmd: f.Cmd})
			}
			if err != nil {
				logger.Logrus().WithError(err).Error("auto add job error")
				return nil, err
			} else {
				logger.Logrus().Traceln("auto add job success jobId:", funcId)
			}
		}
	}
	if !c.ManualStart {
		instance.Start()
	}
	return instance, nil
}

func (c *CronModule) Unregister(maxWaitSeconds uint) (bool, error) {
	ctx := instance.Stop()
	select {
	case <-ctx.Done():
		logger.Logrus().Traceln("")
		return true, nil
	case <-time.After(time.Second * time.Duration(maxWaitSeconds)):
		return false, errors.New("wait too long")
	}
}

// Start 启动已注册任务
func Start() {
	instance.Start()
}

// AddJob 添加任务
func AddJob(spec string, cmd func()) (cron.EntryID, error) {
	return instance.AddFunc(spec, cmd)
}

// AddSingletonJob 添加单例任务 该任务将忽略正在运行的任务的调度
func AddSingletonJob(spec string, cmd func()) (cron.EntryID, error) {
	return instance.AddJob(spec, &singleJob{cmd: cmd})
}

func RawInstance() *cron.Cron {
	return instance
}
