package logger

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	file := "../../logs/logtst.log"
	defer os.Remove(file)
	logg := New(file, "INFO")
	logg.Debug("Debug logging")
	logg.Info("Info logging")
	logg.Warn("Warn logging")
	logg.Error("Error logging")

	f, err := os.Open(file)
	if err != nil {
		fmt.Println()
	}
	defer f.Close()
	content, _ := ioutil.ReadAll(f)
	require.Containsf(t, string(content), "Info logging", "Error logs")
	require.Containsf(t, string(content), "Warn logging", "Error logs")
	require.Containsf(t, string(content), "Error logging", "Error logs")
	require.NotContainsf(t, string(content), "Debug logging", "Error level logs")
}
