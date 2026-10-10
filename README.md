# starter-cron

`starter-cron` is the scheduled job starter for the golang-acexy starter/cloud ecosystem. It wraps `github.com/robfig/cron/v3` and provides starter-managed lifecycle, job registration, singleton job execution, and dynamic schedule refresh.

## Ecosystem Role

This starter owns scheduled-task infrastructure. Business code registers jobs through its focused APIs, while `starter-parent` controls scheduler startup and shutdown with the rest of the application.

## Requirements

Current module Go version: `1.26.7`.

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

## Core Configuration

| Field | Behavior |
| --- | --- |
| `EnableLogger` | Enables scheduler logs; callback panic logging is independent |
| `Location` | Scheduler time zone; nil uses `time.Local` |
| `WithSeconds` | Uses six-field expressions instead of the default five-field form; descriptors such as `@every` remain supported |
| `ManualStart` | Delays scheduling until package-level `Start()` |

`LazyConfig` takes precedence over `Config` and is evaluated once on first startup. Keep configuration fixed after startup. `CronSetting` overrides the default parent lifecycle setting.

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
if err := job.FlushSpec("@every 5m"); err != nil {
    // Report the failed update.
}
```

Use a callback-returned expression for completion-driven updates:

```go
err := cronstarter.NewJobAndRegisterWithNewSpec("dynamic-job", "@every 1m", func() string {
    // Perform work, then return the next schedule; an empty result keeps the current one.
    return "@every 2m"
})
if err != nil {
    panic(err)
}
```

`NewJobAndRegisterWithNewSpec` rejects a nil callback with `ErrJobFuncNil` before registering anything. With `NewJob(..., &spec, true, ...)`, the expression is borrowed and checked after successful callback completion. Do not write to that string concurrently with task execution or registration. Automatic reload is not guaranteed safe alongside manual updates or overlapping callbacks; avoid `multiRun=true` for callback-driven updates.

`FlushSpec` disables automatic reload. A nonblank but invalid new expression can remove the old schedule before returning an error. Automatic replacement failures are logged and can also leave the task unregistered. Removing or replacing a schedule does not cancel a running callback; the replacement may run before an old callback finishes.

## Common API

- `CronStarter.Start()` initializes the cron runtime and optionally starts scheduling.
- `CronStarter.Stop(maxWaitTime)` gracefully stops the scheduler.
- `Start()` manually starts scheduling when `ManualStart` is enabled.
- `RawCron()` returns the underlying `*cron.Cron`.
- `NewJob(...)` creates a named job configuration.
- `NewJobAndRegister(...)` creates and registers a named job.
- `NewJobAndRegisterWithNewSpec(...)` registers a callback that returns its next expression.
- `RemoveJob(name)` removes a registered named job and logs failures.
- `job.Remove()` and `job.FlushSpec(...)` return removal or update errors.

## Lifecycle and Design Notes

Register jobs only after the starter has been initialized by parent. Calls made before startup return `ErrCronStarterNotStarted`.

The scheduler is process-wide. Coordinate job registration, manual start, and updates with shutdown. Calls resolving the scheduler after shutdown begins fail, but an operation already using the old scheduler can continue. A shutdown timeout does not restart scheduling; a new starter cannot start until already-running jobs have exited. `ManualStart` delays scheduling but does not bypass parent initialization. The standard Cron starter does not allow parent-managed restart after successful shutdown.

Named jobs require a non-empty name, non-empty schedule, and non-nil callback. Simple jobs require a non-empty schedule and non-nil callback.

Named task handles operate by name. After removing a task and reusing its name, discard the old handle: it can remove or replace the new entry. `RawCron()` returns nil when the runtime is unavailable; raw entry operations bypass named-job bookkeeping.

Simple ordinary jobs allow overlapping runs. Named jobs and simple singleton jobs skip overlapping triggers within the current registration by default; named jobs use only the first optional `multiRun` value. This is process-local behavior, not a distributed lock. Callback panics are logged without retrying or rolling back application effects.

On shutdown timeout, `Stop` returns `gracefully=false`, `stopped=true`, and `ErrCronStopTimeout`; running callbacks continue. Callbacks receive no cancellation context from this package, so applications must supply cancellation when needed.
