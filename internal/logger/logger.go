package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// New ...
func New(file, level string) *logrus.Logger {
	logg := logrus.New()

	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err == nil {
		logg.SetOutput(f)
	} else {
		logg.Error("failed to log to file, using default stderr: ", err)
	}

	lvl, err := logrus.ParseLevel(level)
	if err == nil {
		logg.SetLevel(lvl)
	} else {
		logg.Error("failed parse level logger: ", err)
		logg.SetLevel(logrus.DebugLevel)
	}

	logg.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:          true,
		TimestampFormat:        "01/02 15:04:05",
		DisableColors:          true,
		DisableLevelTruncation: false,
		PadLevelText:           true,
		DisableSorting:         true,
	})

	logg.Debug("start logger")

	return logg
}
