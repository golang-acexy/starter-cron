package cronstrater

import (
	"errors"
	"github.com/golang-acexy/starter-parent/parent"
	"github.com/robfig/cron/v3"
	"time"
)

var cronInstance *cron.Cron

type CronStarter struct {
	// 启动详细日志
	EnableLogger bool

	// 手动启动定时任务
	// 如果手动启动需要手动调用cronstrater.Start()方法启动整个任务执行器
	ManualStart bool

	CornSetting *parent.Setting
}

func (c *CronStarter) Setting() *parent.Setting {
	if c.CornSetting != nil {
		return c.CornSetting
	}
	return parent.NewSetting("Cron-Starter", 10, false, time.Second*20, nil)
}

func (c *CronStarter) Start() (interface{}, error) {
	opts := make([]cron.Option, 0)
	if c.EnableLogger {
		opts = append(opts, cron.WithLogger(ll))
	}
	cronInstance = cron.New(opts...)
	if !c.ManualStart {
		cronInstance.Start()
	}
	return cronInstance, nil
}

func (c *CronStarter) Stop(maxWaitTime time.Duration) (gracefully, stopped bool, err error) {
	ctx := cronInstance.Stop()
	select {
	case <-ctx.Done():
		return true, true, nil
	case <-time.After(maxWaitTime):
		cronInstance.Start()
		return false, true, errors.New("waiting for cron starter shutdown timeout")
	}
}

// Start 启动已注册任务 如果CronModule.ManualStart = true时一定需要手动开启
func Start() {
	cronInstance.Start()
}

// RawCron 获取原始的cron实例
func RawCron() *cron.Cron {
	return cronInstance
}
