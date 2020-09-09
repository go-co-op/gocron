package gocron

type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	Debug(args ...interface{})
	Info(args ...interface{})
	Error(args ...interface{})
}

var (
	logger Logger
)

func initLogger(l Logger) {
	logger = l
}

func debugf(format string, args ...interface{}) {
	if logger != nil {
		logger.Debugf(format, args...)
	}
}

func debug(args ...interface{}) {
	if logger != nil {
		logger.Debug(args...)
	}
}

func infof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, args...)
	}
}

func info(args ...interface{}) {
	if logger != nil {
		logger.Info(args...)
	}
}

func errf(format string, args ...interface{}) {
	if logger != nil {
		logger.Errorf(format, args...)
	}
}

func err(args ...interface{}) {
	if logger != nil {
		logger.Error(args...)
	}
}
