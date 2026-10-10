package cronstarter

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-acexy/starter-parent/parent"
	"github.com/robfig/cron/v3"
)

var cronInstance atomic.Pointer[cron.Cron]
var cronLifecycleLock sync.Mutex
var cronStopping atomic.Bool

// CronConfig configures the process-wide scheduler; keep configuration fixed after startup.
type CronConfig struct {
	// EnableLogger enables the Cron logging adapter; callback panic logging is independent.
	EnableLogger bool
	// Location selects the scheduler time zone; nil uses the Cron default (time.Local).
	Location *time.Location
	// WithSeconds requires the seconds field in ordinary expressions; descriptors remain supported.
	WithSeconds bool
	// ManualStart publishes the scheduler without starting scheduling. Call package Start after registration.
	ManualStart bool
}

// CronStarter owns the process-wide scheduler. Configure it before startup;
// concurrent mutation of its configuration fields is not supported.
type CronStarter struct {
	// Config supplies static configuration when LazyConfig is nil.
	Config CronConfig
	// LazyConfig takes precedence over Config and is evaluated once on first startup.
	LazyConfig func() CronConfig
	config     *CronConfig
	configOnce sync.Once
	// CronSetting overrides the default non-restartable parent lifecycle setting.
	CronSetting *parent.Setting
}

func (c *CronStarter) getConfig() *CronConfig {
	c.configOnce.Do(func() {
		var config CronConfig
		if c.LazyConfig != nil {
			config = c.LazyConfig()
		} else {
			config = c.Config
		}
		c.config = &config
	})
	return c.config
}

// Setting returns the custom setting or the default priority-10, non-restartable parent setting.
func (c *CronStarter) Setting() *parent.Setting {
	if c.CronSetting != nil {
		return c.CronSetting
	}
	return parent.NewSetting("Cron-Starter", false, 10, false, time.Second*20, nil)
}

// Start initializes the singleton scheduler and starts scheduling unless ManualStart is true.
// An existing scheduler or draining prior scheduler returns ErrCronStarterAlreadyStarted.
func (c *CronStarter) Start() (any, error) {
	cronLifecycleLock.Lock()
	defer cronLifecycleLock.Unlock()
	if cronInstance.Load() != nil || cronStopping.Load() {
		return cronInstance.Load(), ErrCronStarterAlreadyStarted
	}
	config := c.getConfig()
	opts := make([]cron.Option, 0)
	if config.EnableLogger {
		opts = append(opts, cron.WithLogger(log))
	}
	if config.Location != nil {
		opts = append(opts, cron.WithLocation(config.Location))
	}
	if config.WithSeconds {
		opts = append(opts, cron.WithSeconds())
	}
	instance := cron.New(opts...)
	cronInstance.Store(instance)
	if !config.ManualStart {
		instance.Start()
	}
	return instance, nil
}

// Stop withdraws the scheduler, clears named entries, and waits up to maxWaitTime for callbacks.
// Timeout returns false, true, ErrCronStopTimeout; callbacks continue and block new startup until done.
// Nonpositive waits expire immediately; if completion is also ready, either outcome can be selected.
// Coordinate job API calls with shutdown; previously captured scheduler pointers remain usable.
func (c *CronStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	cronLifecycleLock.Lock()
	instance := cronInstance.Swap(nil)
	if instance == nil {
		cronLifecycleLock.Unlock()
		return false, true, ErrCronStarterNotStarted
	}
	cronStopping.Store(true)
	cronLifecycleLock.Unlock()
	ctx := instance.Stop()
	jobListLock.Lock()
	jobList = make(map[string]*jobInfo)
	jobListLock.Unlock()
	timer := time.NewTimer(maxWaitTime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		cronStopping.Store(false)
		return true, true, nil
	case <-timer.C:
		go func() {
			<-ctx.Done()
			cronStopping.Store(false)
		}()
		return false, true, ErrCronStopTimeout
	}
}

// Start begins scheduling on the initialized singleton, including when ManualStart is true.
// Repeated calls on a running scheduler are harmless; an absent runtime returns ErrCronStarterNotStarted.
// Coordinate calls with starter shutdown and do not restart a retained raw instance.
func Start() error {
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	instance.Start()
	return nil
}

// RawCron returns the borrowed scheduler, or nil before initialization and after runtime withdrawal.
// Raw operations bypass named-job bookkeeping; do not retain and restart it after starter shutdown.
func RawCron() *cron.Cron {
	return cronInstance.Load()
}

func getCronInstance() (*cron.Cron, error) {
	instance := cronInstance.Load()
	if instance == nil {
		return nil, ErrCronStarterNotStarted
	}
	return instance, nil
}
