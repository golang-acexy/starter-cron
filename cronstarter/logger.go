package cronstarter

import "github.com/acexy/golang-toolkit/logger"

var log = &logrusLogger{}

type logrusLogger struct {
}

func (*logrusLogger) Info(msg string, keysAndValues ...interface{}) {
	logger.Logrus().Info(msg, keysAndValues)
}

func (*logrusLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	logger.Logrus().WithError(err).Error(msg, keysAndValues)
}
