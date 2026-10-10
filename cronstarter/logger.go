package cronstarter

import "github.com/acexy/golang-toolkit/logger"

var log = &logrusLogger{}

type logrusLogger struct {
}

// Info forwards scheduler events to the shared application logger.
func (*logrusLogger) Info(msg string, keysAndValues ...any) {
	logger.Logrus().Info(msg, keysAndValues)
}

// Error forwards scheduler failures with the supplied error attached.
func (*logrusLogger) Error(err error, msg string, keysAndValues ...any) {
	logger.Logrus().WithError(err).Error(msg, keysAndValues)
}
