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
	wrapper *job
}

type job struct {
	m          sync.Mutex
	originSpec string
	jobFunc    *jobFunc
	cmd        func()
}

// Run executes a callback with panic recovery and skips overlapping singleton triggers.
// Successful callbacks apply schedule changes before returning; retired results are ignored.
func (j *job) Run() {
	defer func() {
		if err := recover(); err != nil {
			name := "unnamed"
			if j.jobFunc != nil {
				name = j.jobFunc.jobName
			}
			logger.Logrus().Errorln("job run error", err, "job name:", name, filterStack(string(debug.Stack())))
		}
	}()
	if j.jobFunc == nil {
		if !j.m.TryLock() {
			return
		}
		defer j.m.Unlock()
		j.cmd()
		return
	}
	owner := j.jobFunc
	if !owner.multiRun {
		if !owner.run.TryLock() {
			return
		}
		defer owner.run.Unlock()
	}
	if owner.nextSpec != nil {
		next := owner.nextSpec()
		owner.reload(j, next, true)
	} else {
		j.cmd()
		owner.reload(j, "", false)
	}
}

type jobFunc struct {
	sync.Mutex
	run            sync.Mutex
	jobName        string
	multiRun       bool
	spec           *string
	cmd            func()
	nextSpec       func() string
	autoReloadSpec bool
}

// NewJob creates an unregistered named job; Register validates its inputs.
// It borrows spec: external writes must not overlap registration or automatic schedule checks.
// Only the first multiRun value is used. By default, one handle skips overlapping callbacks
// across replacements. Removal and replacement do not cancel callbacks already running.
func NewJob(jobName string, spec *string, autoReloadSpec bool, cmd func(), multiRun ...bool) *jobFunc {
	j := &jobFunc{jobName: jobName, spec: spec, cmd: cmd, autoReloadSpec: autoReloadSpec}
	if len(multiRun) > 0 {
		j.multiRun = multiRun[0]
	}
	return j
}

// NewJobAndRegister creates and registers a named job after starter initialization.
// It has the borrowed-spec and overlap constraints of NewJob; duplicate names return an error.
func NewJobAndRegister(jobName string, spec *string, autoReloadSpec bool, cmd func(), multiRun ...bool) error {
	return NewJob(jobName, spec, autoReloadSpec, cmd, multiRun...).Register()
}

// NewJobAndRegisterWithNewSpec registers a callback-driven schedule after initialization.
// Nil callbacks return ErrJobFuncNil. Empty results keep the current schedule; invalid results
// preserve the old entry and are logged. Internal updates support overlapping callbacks.
// Results from superseded registrations are ignored. Only the first multiRun value is used.
func NewJobAndRegisterWithNewSpec(jobName, spec string, cmd func() string, multiRun ...bool) error {
	if cmd == nil {
		return fmt.Errorf("%w: %s", ErrJobFuncNil, jobName)
	}
	j := NewJob(jobName, &spec, true, nil, multiRun...)
	j.nextSpec = cmd
	return j.Register()
}

// RemoveJob removes the currently registered name and logs missing entries or removal failures.
// It does not cancel running callbacks or return an error; simple entry IDs are not named jobs.
func RemoveJob(jobName string) {
	jobListLock.Lock()
	j, ok := jobList[jobName]
	jobListLock.Unlock()
	if !ok {
		logger.Logrus().Warning("job not exists:", jobName)
		return
	}
	if err := j.jobFunc.Remove(); err != nil {
		logger.Logrus().WithError(err).Errorln("remove job error", jobName, err)
	}
}

// Register validates and registers this handle; duplicate names fail.
// Registration is coordinated with starter shutdown. Keep the borrowed expression stable.
func (j *jobFunc) Register() error {
	j.Lock()
	defer j.Unlock()
	if j.spec == nil {
		return fmt.Errorf("%w: %s", ErrJobSpecNil, j.jobName)
	}
	if strings.TrimSpace(j.jobName) == "" {
		return ErrJobNameEmpty
	}
	if strings.TrimSpace(*j.spec) == "" {
		return fmt.Errorf("%w: %s", ErrJobSpecEmpty, j.jobName)
	}
	if j.cmd == nil && j.nextSpec == nil {
		return fmt.Errorf("%w: %s", ErrJobFuncNil, j.jobName)
	}
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	if _, ok := jobList[j.jobName]; ok {
		return fmt.Errorf("%w: %s", ErrJobAlreadyExists, j.jobName)
	}
	wrapper := &job{cmd: j.cmd, originSpec: *j.spec, jobFunc: j}
	id, err := instance.AddJob(*j.spec, wrapper)
	if err != nil {
		return err
	}
	jobList[j.jobName] = &jobInfo{jobId: id, jobFunc: j, wrapper: wrapper}
	return nil
}

// FlushSpec replaces this handle's entry and disables automatic reload on success.
// Invalid expressions preserve the old entry and settings. An absent entry or a name owned by
// another handle returns ErrJobNotExists. Singleton exclusion is shared across replacements.
func (j *jobFunc) FlushSpec(spec string) error {
	j.Lock()
	defer j.Unlock()
	if err := j.replaceLocked(spec, nil); err != nil {
		return err
	}
	j.autoReloadSpec = false
	return nil
}

// replaceLocked keeps the old entry until parsing and adding its replacement succeeds.
// The caller holds j.Mutex; source additionally identifies an automatic update's registration.
func (j *jobFunc) replaceLocked(spec string, source *job) error {
	if strings.TrimSpace(spec) == "" {
		return fmt.Errorf("%w: %s", ErrJobSpecEmpty, j.jobName)
	}
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	current, ok := jobList[j.jobName]
	if !ok || current.jobFunc != j {
		return fmt.Errorf("%w: %s", ErrJobNotExists, j.jobName)
	}
	if source != nil && current.wrapper != source {
		return nil
	}
	wrapper := &job{cmd: j.cmd, originSpec: spec, jobFunc: j}
	id, err := instance.AddJob(spec, wrapper)
	if err != nil {
		return err
	}
	instance.Remove(current.jobId)
	jobList[j.jobName] = &jobInfo{jobId: id, jobFunc: j, wrapper: wrapper}
	// Automatic borrowed-pointer updates keep the original pointer. Manual updates detach it.
	if source == nil || j.nextSpec != nil {
		j.spec = &spec
	}
	return nil
}

// reload applies completed callback results without spawning untracked refresh goroutines.
func (j *jobFunc) reload(source *job, next string, returned bool) {
	j.Lock()
	defer j.Unlock()
	if !j.autoReloadSpec {
		return
	}
	if returned {
		if next == "" {
			return
		}
	} else {
		if j.spec == nil {
			return
		}
		next = *j.spec
	}
	if next == source.originSpec {
		return
	}
	if err := j.replaceLocked(next, source); err != nil {
		logger.Logrus().WithError(err).Errorln("auto flush spec failed")
	}
}

// Remove deletes only this handle's entry without canceling running callbacks.
// Missing entries and names now owned by another handle return ErrJobNotExists.
func (j *jobFunc) Remove() error {
	j.Lock()
	defer j.Unlock()
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	jobListLock.Lock()
	defer jobListLock.Unlock()
	current, ok := jobList[j.jobName]
	if !ok || current.jobFunc != j {
		return fmt.Errorf("%w: %s", ErrJobNotExists, j.jobName)
	}
	instance.Remove(current.jobId)
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
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
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
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
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
