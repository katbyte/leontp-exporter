// Package main implements leontp-metrics, which reports what a LeoNTP time server says about itself as a Prometheus metrics page.
package main

import (
	"os"

	"github.com/gookit/color"
	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/leontp-metrics/cli"
)

func main() {
	// the log level comes from LEONTP_METRICS_LOG; read it once here, before anything logs
	clog.SetLevelFromEnv("LEONTP_METRICS_LOG")

	cmd, err := cli.Make()
	if err != nil {
		clog.Log.Error(color.Sprintf("<red>leontp-metrics: building cmd</> %v", err))

		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		clog.Log.Error(color.Sprintf("<red>leontp-metrics:</> %v", err))

		os.Exit(1)
	}

	os.Exit(0)
}
