package cronstarter

import "errors"

var (
	ErrCronStarterNotStarted = errors.New("cron starter not started")
	ErrCronStarterAlreadyStarted = errors.New("cron starter already started")
	ErrCronStopTimeout       = errors.New("waiting for cron starter shutdown timeout")
	ErrJobSpecNil            = errors.New("job spec is nil")
	ErrJobSpecEmpty          = errors.New("job spec must not be empty")
	ErrJobNameEmpty          = errors.New("job name must not be empty")
	ErrJobFuncNil            = errors.New("job func must not be nil")
	ErrJobAlreadyExists      = errors.New("the job already exists")
	ErrJobNotExists          = errors.New("the job not exists")
)
