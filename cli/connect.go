package cli

import (
	"github.com/katbyte/leontp-exporter/lib/readings"
	"github.com/katbyte/leontp-exporter/sdk/leontp"
)

// connect makes the client that asks the time server its questions.
func connect(server string, o leontp.Options) (readings.Server, error) {
	return leontp.New(server, o)
}
