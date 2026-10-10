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

// Run executes a callback and logs recovered panics; singleton wrappers skip overlapping triggers.
// Automatic schedule checks occur only after the callback returns normally.
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

	// jobName identifies the named registry entry.
	jobName string

	// multiRun permits overlapping callback invocations.
	multiRun bool

	// spec is a caller-owned expression pointer read during registration and reload.
	spec *string

	// cmd contains the application callback.
	cmd func()

	// autoReloadSpec checks for expression changes after successful callback completion.
	autoReloadSpec bool
}

// NewJob creates an unregistered named job; Register validates its inputs.
// It borrows spec. Only the first multiRun value is used; false defaults to skipping overlaps
// within each execution wrapper. Replacement creates a new wrapper and does not drain the old one.
// Schedule reads are not synchronized with external writes or concurrent FlushSpec calls;
// do not assume automatic reload is concurrency-safe.
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

// NewJobAndRegister creates and registers a named job after starter initialization.
// It has the borrowed-spec and overlap constraints of NewJob; duplicate names return an error.
func NewJobAndRegister(jobName string, spec *string, autoReloadSpec bool, cmd func(), multiRun ...bool) error {
	return NewJob(jobName, spec, autoReloadSpec, cmd, multiRun...).Register()
}

// NewJobAndRegisterWithNewSpec registers a callback-driven schedule after starter initialization.
// A nil callback returns ErrJobFuncNil without registering. An empty callback result keeps the
// current expression; nonempty results trigger asynchronous replacement after normal completion.
// Only the first multiRun value is used; overlapping callbacks can race on the internal expression.
// Automatic replacement errors are logged and can leave the job unregistered.
func NewJobAndRegisterWithNewSpec(jobName string, spec string, cmd func() string, multiRun ...bool) error {
	if cmd == nil {
		return fmt.Errorf("%w: %s", ErrJobFuncNil, jobName)
	}
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

// RemoveJob removes the currently registered name and logs missing entries or removal failures.
// It does not cancel running callbacks or return an error; simple entry IDs are not named jobs.
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

// Register validates inputs and adds the job to the current scheduler; duplicate names fail.
// The starter must be initialized. Keep the borrowed expression stable during registration.
// Lifecycle withdrawal is not atomic with registration after the scheduler pointer is captured.
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

// FlushSpec replaces the entry registered under this job's name and disables automatic reload.
// Blank expressions are rejected before removal; a parser error can leave the old entry removed.
// It does not verify ownership of a reused name or cancel running callbacks. The replacement
// gets a new overlap lock. Do not call it concurrently with callback-driven schedule checks.
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

// Remove deletes the entry registered under this job's name without canceling running callbacks.
// It returns ErrJobNotExists when absent and does not verify ownership of a reused name.
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

// AddSimpleJob registers an unnamed callback after starter initialization and allows overlap.
// Blank expressions and nil callbacks are rejected; parser errors are returned.
// Panics are logged without retry. Manage its returned entry ID through RawCron, not RemoveJob.
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

// AddSimpleSingletonJob registers an unnamed callback and skips overlapping triggers in this process.
// Blank expressions and nil callbacks are rejected; parser errors are returned.
// Panics are logged without retry. Its entry ID is not part of the named-job registry.
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
