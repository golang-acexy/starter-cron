package cronstarter

import (
	"sync"
	"time"

	"github.com/golang-acexy/starter-parent/parent"
	"github.com/robfig/cron/v3"
)

var cronInstance *cron.Cron
var cronInstanceLock sync.RWMutex

type CronConfig struct {
	// 启动详细日志
	EnableLogger bool

	// 手动启动定时任务
	// 如果手动启动需要手动调用cronstarter.Start()方法启动整个任务执行器
	ManualStart bool
}

type CronStarter struct {
	Config      CronConfig
	LazyConfig  func() CronConfig
	config      *CronConfig
	CronSetting *parent.Setting
}

func (c *CronStarter) getConfig() *CronConfig {
	if c.config == nil {
		var config CronConfig
		if c.LazyConfig != nil {
			config = c.LazyConfig()
		} else {
			config = c.Config
		}
		c.config = &config
	}
	return c.config
}

func (c *CronStarter) Setting() *parent.Setting {
	if c.CronSetting != nil {
		return c.CronSetting
	}
	return parent.NewSetting("Cron-Starter", 10, false, time.Second*20, nil)
}

func (c *CronStarter) Start() (any, error) {
	config := c.getConfig()
	opts := make([]cron.Option, 0)
	if config.EnableLogger {
		opts = append(opts, cron.WithLogger(log))
	}
	instance := cron.New(opts...)
	cronInstanceLock.Lock()
	cronInstance = instance
	cronInstanceLock.Unlock()
	if !config.ManualStart {
		instance.Start()
	}
	return instance, nil
}

func (c *CronStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	instance, err := getCronInstance()
	if err != nil {
		return false, true, err
	}
	ctx := instance.Stop()
	select {
	case <-ctx.Done():
		return true, true, nil
	case <-time.After(maxWaitTime):
		instance.Start()
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
	cronInstanceLock.RLock()
	defer cronInstanceLock.RUnlock()
	return cronInstance
}

func getCronInstance() (*cron.Cron, error) {
	cronInstanceLock.RLock()
	defer cronInstanceLock.RUnlock()
	if cronInstance == nil {
		return nil, ErrCronStarterNotStarted
	}
	return cronInstance, nil
}
