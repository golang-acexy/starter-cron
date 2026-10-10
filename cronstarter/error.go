package cronstarter

import "errors"

var (
	// ErrCronStarterNotStarted reports that no scheduler is currently published.
	ErrCronStarterNotStarted = errors.New("cron starter not started")
	// ErrCronStarterAlreadyStarted reports an existing scheduler or a prior scheduler still draining.
	ErrCronStarterAlreadyStarted = errors.New("cron starter already started")
	// ErrCronStopTimeout reports that scheduling stopped but callbacks did not finish within the wait.
	ErrCronStopTimeout = errors.New("waiting for cron starter shutdown timeout")
	// ErrJobSpecNil reports a nil named-job expression pointer.
	ErrJobSpecNil = errors.New("job spec is nil")
	// ErrJobSpecEmpty reports a blank scheduling expression.
	ErrJobSpecEmpty = errors.New("job spec must not be empty")
	// ErrJobNameEmpty reports a blank named-job identifier.
	ErrJobNameEmpty = errors.New("job name must not be empty")
	// ErrJobFuncNil reports a nil application callback; registration is rejected.
	ErrJobFuncNil = errors.New("job func must not be nil")
	// ErrJobAlreadyExists reports a duplicate registered job name.
	ErrJobAlreadyExists = errors.New("the job already exists")
	// ErrJobNotExists reports a missing named registry entry.
	ErrJobNotExists = errors.New("the job not exists")
)
