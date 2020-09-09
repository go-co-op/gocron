package gocron

// Logger interface the custom logger should
// conform to
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Errorf(format string, args ...interface{})

	Debug(args ...interface{})
	Info(args ...interface{})
	Error(args ...interface{})
}

// OptionLogger returns the SchedulerOption for
// for passing in a custom logger to NewScheduler
func OptionLogger(logger Logger) SchedulerOption {
	return func(s *Scheduler) {
		s.logger = logger
	}
}

func (s *Scheduler) initLogger(l Logger) {
	s.logger = l
}

func (s *Scheduler) debugf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Debugf(format, args...)
	}
}

func (s *Scheduler) debug(args ...interface{}) {
	if s.logger != nil {
		s.logger.Debug(args...)
	}
}

func (s *Scheduler) infof(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Infof(format, args...)
	}
}

func (s *Scheduler) info(args ...interface{}) {
	if s.logger != nil {
		s.logger.Info(args...)
	}
}

func (s *Scheduler) errorf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}

func (s *Scheduler) error(args ...interface{}) {
	if s.logger != nil {
		s.logger.Error(args...)
	}
}
