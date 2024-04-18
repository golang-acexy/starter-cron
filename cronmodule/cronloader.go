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

var jobList = make(map[string]*jobInfo)

type jobInfo struct {
	jobId   cron.EntryID
	jobFunc *JobFunc
}

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
	sync.Mutex
	// 唯一任务名称
	JobName string
	// 是否允许同一个任务在上一个调度未完成时继续被调度执行
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

	// 初始化的任务 初始化的任务不提供后续管理功能
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
		for i := 0; i < len(c.Func); i++ {
			f := &c.Func[i]
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

// Start 启动已注册任务 如果CronModule.ManualStart = true时一定需要手动开启
func Start() {
	instance.Start()
}

// NewJob 初始化一个Job配置
func NewJob(jobName string, spec string, cmd func(), multiRun ...bool) *JobFunc {
	j := &JobFunc{
		JobName: jobName,
		Spec:    spec,
		Cmd:     cmd,
	}
	if len(multiRun) > 0 && multiRun[0] {
		j.MultiRun = true
	}
	return j
}

// Register 注册该Job
func (j *JobFunc) Register() error {
	defer j.Unlock()
	j.Lock()
	_, flag := jobList[j.JobName]
	if flag {
		return errors.New("the job already exists : " + j.JobName)
	}
	var id cron.EntryID
	var err error
	if j.MultiRun {
		id, err = instance.AddFunc(j.Spec, j.Cmd)
	} else {
		id, err = instance.AddJob(j.Spec, &singleJob{cmd: j.Cmd})
	}
	if err != nil {
		return err
	}
	jobList[j.JobName] = &jobInfo{
		jobId:   id,
		jobFunc: j,
	}
	return nil
}

// FlushSpec 更改Job规则
func (j *JobFunc) FlushSpec(spec string) error {
	j.Lock()
	v, flag := jobList[j.JobName]
	if !flag {
		return errors.New("the job not exists : " + j.JobName)
	}
	j.Unlock()
	j.Spec = spec
	instance.Remove(v.jobId)
	delete(jobList, j.JobName)
	return j.Register()
}

// Remove 移除任务
func (j *JobFunc) Remove() error {
	defer j.Unlock()
	j.Lock()
	v, flag := jobList[j.JobName]
	if !flag {
		return errors.New("the job not exists : " + j.JobName)
	}
	instance.Remove(v.jobId)
	delete(jobList, j.JobName)
	return nil
}

// AddSimpleJob 添加简单任务
func AddSimpleJob(spec string, cmd func()) (cron.EntryID, error) {
	return instance.AddFunc(spec, cmd)
}

// AddSimpleSingletonJob 添加简单单例任务 该任务将忽略正在运行的任务的调度
func AddSimpleSingletonJob(spec string, cmd func()) (cron.EntryID, error) {
	return instance.AddJob(spec, &singleJob{cmd: cmd})
}

func RawInstance() *cron.Cron {
	return instance
}
