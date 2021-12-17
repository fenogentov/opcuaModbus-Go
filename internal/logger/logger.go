package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger ...
type Logger struct {
	logger *logrus.Logger
}

// New ...
func New(file, level string) *Logger {
	logg := logrus.New()

	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err == nil {
		logg.SetOutput(f)
	} else {
		logg.Info("Failed to log to file, using default stderr: ", err)
	}

	lvl, err := logrus.ParseLevel(level)
	if err == nil {
		logg.SetLevel(lvl)
	} else {
		logg.Info("Failed parse level logger", err)
		logg.SetLevel(logrus.DebugLevel)
	}

	logg.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "01/02 15:04:05",
		DisableColors:   true,
		PadLevelText:    true,
	})
	logg.Debug("start log")
	return &Logger{logger: logg}
}

// Info ...
func (l Logger) Info(msg string) {
	l.logger.Info(msg)
}

// Error ...
func (l Logger) Error(msg string) {
	l.logger.Error(msg)
}

// Warn ...
func (l Logger) Warn(msg string) {
	l.logger.Warn(msg)
}

// Debug ...
func (l Logger) Debug(msg string) {
	l.logger.Debug(msg)
}
