package leontp

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// statsRequest is the request code of the statistics question, the one question the maker's script asks.
const statsRequest = 1

// Stats is a LeoNTP's own account of itself. The layout is the one in the maker's script, kept in api-defs.
type Stats struct {
	// Time is the server's clock when it answered.
	Time time.Time
	// Uptime is how long it has been powered on.
	Uptime time.Duration
	// Requests is how many NTP requests it has answered since it was powered on. A question about its statistics is not one of them.
	Requests uint32
	// ControlRequests is how many ntpq questions it has answered since it was powered on. A 1200 answers none, so this stays 0 there.
	ControlRequests uint32
	// LockTime is how long the GPS has been locked, which starts again each time the fix is lost and found.
	LockTime time.Duration
	// Satellites is how many satellites the GPS receiver is using.
	Satellites int
	// Flags is a status byte the maker does not document: 1 on a healthy, locked 1200.
	Flags uint8
	// Serial is the unit's serial number.
	Serial int
	// Firmware is the firmware version as the front panel shows it, "2.07" say.
	Firmware string
	// FirmwareWord is the four bytes the version was read from. On a 1200 the version is all they hold; the older unit keeps something above it
	// that the maker does not document.
	FirmwareWord uint32
}

// AverageRate is the NTP requests answered each second since the server was powered on, 0 for one that has only just been.
func (s Stats) AverageRate() float64 {
	if s.Uptime <= 0 {
		return 0
	}

	return float64(s.Requests) / s.Uptime.Seconds()
}

// Stats asks the server for its statistics.
func (c *Client) Stats(ctx context.Context) (Stats, error) {
	answer, err := c.private(ctx, statsRequest)
	if err != nil {
		return Stats{}, fmt.Errorf("asking %s for its statistics: %w", c.addr, err)
	}

	return parseStats(answer), nil
}

// parseStats reads a whole answer to the statistics question. What follows its 16 byte header is written low byte first, unlike the rest of NTP.
func parseStats(b []byte) Stats {
	le := binary.LittleEndian
	firmware := le.Uint32(b[44:48])

	return Stats{
		Time:            ntpTime(le.Uint32(b[20:24]), le.Uint32(b[16:20])),
		Uptime:          time.Duration(le.Uint32(b[24:28])) * time.Second,
		Requests:        le.Uint32(b[28:32]),
		ControlRequests: le.Uint32(b[32:36]),
		LockTime:        time.Duration(le.Uint32(b[36:40])) * time.Second,
		Flags:           b[40],
		Satellites:      int(b[41]),
		Serial:          int(le.Uint16(b[42:44])),
		Firmware:        fmt.Sprintf("%x.%02x", firmware>>8&0xff, firmware&0xff),
		FirmwareWord:    firmware,
	}
}
