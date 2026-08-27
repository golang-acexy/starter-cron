package cronstarter

import (
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/acexy/golang-toolkit/logger"
	"github.com/acexy/golang-toolkit/util/coll"
	"github.com/robfig/cron/v3"
)

var jobListLock sync.Mutex

var jobList = make(map[string]*jobInfo)

func filterStack(stack string) string {
	lines := strings.Split(stack, "\n")
	start := coll.SliceIndexBy(lines, func(line string) bool {
		return strings.Contains(line, "runtime/panic.go")
	})
	if start < 0 {
		start = 0
	}
	filtered := lines[start:]
	end := coll.SliceIndexBy(filtered, func(line string) bool {
		return strings.Contains(line, "cronstarter/job.go")
	})
	if end <= 0 {
		return strings.Join(filtered, "\n")
	}
	return strings.Join(filtered[:end], "\n")
}

type jobInfo struct {
	jobId   cron.EntryID
	jobFunc *jobFunc
}

type job struct {
	m sync.Mutex

	originSpec string
	jobFunc    *jobFunc

	cmd func()
}

func (j *job) Run() {
	defer func() {
		if err := recover(); err != nil {
			jobName := "unnamed"
			if j.jobFunc != nil {
				jobName = j.jobFunc.jobName
			}
			logger.Logrus().Errorln("job run error", err, "job name:", jobName, filterStack(string(debug.Stack())))
		}
	}()
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
		if j.jobFunc.autoReloadSpec && j.jobFunc.spec != nil {
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

// NewJobAndRegisterWithNewSpec 初始化一个Job配置 并注册 (刷新spec)
func NewJobAndRegisterWithNewSpec(jobName string, spec string, cmd func() string, multiRun ...bool) error {
	var flushSpec string
	flushSpec = spec
	cmdWrap := func() {
		newSpec := cmd()
		if newSpec != "" {
			flushSpec = newSpec
		}
	}
	return NewJob(jobName, &flushSpec, true, cmdWrap, multiRun...).Register()
}

// RemoveJob 移除任务
func RemoveJob(jobName string) {
	jobListLock.Lock()
	j, flag := jobList[jobName]
	jobListLock.Unlock()
	if !flag {
		logger.Logrus().Warning("job not exists:", jobName)
		return
	}
	err := j.jobFunc.Remove()
	if err != nil {
		logger.Logrus().WithError(err).Errorln("remove job error", jobName, err)
	}
}

// Register 注册该Job
func (j *jobFunc) Register() error {
	defer j.Unlock()
	j.Lock()
	if j.spec == nil {
		return fmt.Errorf("%w: %s", ErrJobSpecNil, j.jobName)
	}
	if strings.TrimSpace(j.jobName) == "" {
		return ErrJobNameEmpty
	}
	if strings.TrimSpace(*j.spec) == "" {
		return fmt.Errorf("%w: %s", ErrJobSpecEmpty, j.jobName)
	}
	if j.cmd == nil {
		return fmt.Errorf("%w: %s", ErrJobFuncNil, j.jobName)
	}
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	_, flag := jobList[j.jobName]
	if flag {
		return fmt.Errorf("%w: %s", ErrJobAlreadyExists, j.jobName)
	}
	id, err := instance.AddJob(*j.spec, &job{
		cmd:        j.cmd,
		originSpec: *j.spec,
		jobFunc:    j,
	})
	if err != nil {
		return err
	}
	jobList[j.jobName] = &jobInfo{
		jobId:   id,
		jobFunc: j,
	}
	return nil
}

// FlushSpec 更改Job规则 该操作将自动关闭 autoReloadSpec
func (j *jobFunc) FlushSpec(spec string) error {
	defer j.Unlock()
	j.Lock()
	if strings.TrimSpace(spec) == "" {
		return fmt.Errorf("%w: %s", ErrJobSpecEmpty, j.jobName)
	}
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	v, flag := jobList[j.jobName]
	if !flag {
		return fmt.Errorf("%w: %s", ErrJobNotExists, j.jobName)
	}
	j.spec = &spec
	j.autoReloadSpec = false
	instance.Remove(v.jobId)
	delete(jobList, j.jobName)
	id, err := instance.AddJob(spec, &job{
		cmd:        j.cmd,
		originSpec: spec,
		jobFunc:    j,
	})
	if err != nil {
		return err
	}
	jobList[j.jobName] = &jobInfo{
		jobId:   id,
		jobFunc: j,
	}
	return nil
}

// Remove 移除任务
func (j *jobFunc) Remove() error {
	defer j.Unlock()
	j.Lock()
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	v, flag := jobList[j.jobName]
	if !flag {
		return fmt.Errorf("%w: %s", ErrJobNotExists, j.jobName)
	}
	instance.Remove(v.jobId)
	delete(jobList, j.jobName)
	return nil
}

// AddSimpleJob 添加简单任务
func AddSimpleJob(spec string, cmd func()) (cron.EntryID, error) {
	if strings.TrimSpace(spec) == "" {
		return 0, ErrJobSpecEmpty
	}
	if cmd == nil {
		return 0, ErrJobFuncNil
	}
	instance, err := getCronInstance()
	if err != nil {
		return 0, err
	}
	var fn = func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Logrus().Errorln("job run error", err, filterStack(string(debug.Stack())))
			}
		}()
		cmd()
	}
	return instance.AddFunc(spec, fn)
}

// AddSimpleSingletonJob 添加简单单例任务 该任务将忽略正在运行的任务的调度
func AddSimpleSingletonJob(spec string, cmd func()) (cron.EntryID, error) {
	if strings.TrimSpace(spec) == "" {
		return 0, ErrJobSpecEmpty
	}
	if cmd == nil {
		return 0, ErrJobFuncNil
	}
	instance, err := getCronInstance()
	if err != nil {
		return 0, err
	}
	var fn = func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Logrus().Errorln("job run error", err, filterStack(string(debug.Stack())))
			}
		}()
		cmd()
	}
	return instance.AddJob(spec, &job{cmd: fn})
}
