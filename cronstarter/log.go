package cronstarter

import "github.com/acexy/golang-toolkit/logger"

var ll = &logrusLoggger{}

type logrusLoggger struct {
}

func (*logrusLoggger) Info(msg string, keysAndValues ...interface{}) {
	logger.Logrus().Info(msg, keysAndValues)
}

func (*logrusLoggger) Error(err error, msg string, keysAndValues ...interface{}) {
	logger.Logrus().WithError(err).Error(msg, keysAndValues)
}
