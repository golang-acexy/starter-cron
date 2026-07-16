package cronstarter

import "errors"

var (
	ErrCronStarterNotStarted = errors.New("cron starter not started")
	ErrCronStopTimeout       = errors.New("waiting for cron starter shutdown timeout")
	ErrJobSpecNil            = errors.New("job spec is nil")
	ErrJobAlreadyExists      = errors.New("the job already exists")
	ErrJobNotExists          = errors.New("the job not exists")
)
