# starter-cron

`starter-cron` is the scheduled job starter for the golang-acexy starter/cloud ecosystem. It wraps `github.com/robfig/cron/v3` and provides starter-managed lifecycle, safe job registration, singleton job execution, and dynamic schedule refresh.

## Ecosystem Role

This starter owns scheduled-task infrastructure. Business code registers jobs through its focused APIs, while `starter-parent` controls scheduler startup and shutdown with the rest of the application.

## Requirements

Current module Go version: `1.25.8`.

## Installation

```bash
go get github.com/golang-acexy/starter-cron
```

## Starter Usage

```go
starter := &cronstarter.CronStarter{
    Config: cronstarter.CronConfig{
        EnableLogger: true,
        ManualStart:  false,
    },
}

loader := parent.InitStarterLoader([]parent.Starter{starter})
if err := loader.Start(); err != nil {
    panic(err)
}
```

Set `ManualStart` to `true` when jobs should be registered first and the scheduler should be started later with `cronstarter.Start()`.

## Register Jobs

Simple job:

```go
_, err := cronstarter.AddSimpleJob("@every 1m", func() {
    // job logic
})
```

Singleton job, which skips a new run when the previous run is still executing:

```go
_, err := cronstarter.AddSimpleSingletonJob("@every 10s", func() {
    // long-running job logic
})
```

Named job with explicit registration:

```go
spec := "@every 1m"
job := cronstarter.NewJob("sync-user", &spec, false, func() {
    // job logic
})

if err := job.Register(); err != nil {
    panic(err)
}
```

## Dynamic Schedule

Use `FlushSpec` for manual schedule updates:

```go
_ = job.FlushSpec("@every 5m")
```

Use `autoReloadSpec` when the job should reload after `spec` changes:

```go
spec := "@every 1m"
_ = cronstarter.NewJobAndRegister("dynamic-job", &spec, true, func() {
    // job logic
})

spec = "@every 2m"
```

## Common API

- `CronStarter.Start()` initializes the cron runtime and optionally starts scheduling.
- `CronStarter.Stop(maxWaitTime)` gracefully stops the scheduler.
- `Start()` manually starts scheduling when `ManualStart` is enabled.
- `RawCron()` returns the underlying `*cron.Cron`.
- `NewJob(...)` creates a named job configuration.
- `NewJobAndRegister(...)` creates and registers a named job.
- `RemoveJob(name)` removes a registered named job.

## Lifecycle and Design Notes

Register jobs only after the starter has been initialized by parent. Calls made before startup return `ErrCronStarterNotStarted`.

The scheduler is process-wide. `ManualStart` delays scheduling but does not bypass parent initialization. The standard Cron starter does not allow parent-managed restart after successful shutdown.
