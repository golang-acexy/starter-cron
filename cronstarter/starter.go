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

type CronConfig struct {
	// 启动详细日志
	EnableLogger bool
	// 时区
	Location *time.Location
	// 启用包含秒字段的 Cron 表达式
	WithSeconds bool

	// 手动启动定时任务
	// 如果手动启动需要手动调用cronstarter.Start()方法启动整个任务执行器
	ManualStart bool
}

type CronStarter struct {
	Config      CronConfig
	LazyConfig  func() CronConfig
	config      *CronConfig
	configOnce  sync.Once
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

func (c *CronStarter) Setting() *parent.Setting {
	if c.CronSetting != nil {
		return c.CronSetting
	}
	return parent.NewSetting("Cron-Starter", false, 10, false, time.Second*20, nil)
}

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

// Start 启动已注册任务 如果CronModule.ManualStart = true 时一定需要手动开启
func Start() error {
	instance, err := getCronInstance()
	if err != nil {
		return err
	}
	instance.Start()
	return nil
}

// RawCron 获取原始的cron实例
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
