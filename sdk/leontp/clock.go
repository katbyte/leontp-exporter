package leontp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// The leap second warnings an NTP server gives.
const (
	// LeapNone is no warning.
	LeapNone = 0
	// LeapAdd says the last minute of the day has a second added.
	LeapAdd = 1
	// LeapDrop says the last minute of the day has a second dropped.
	LeapDrop = 2
	// LeapNotSet says the server's clock is not set.
	LeapNotSet = 3
)

// maxStratum is the stratum of a server with no time to vouch for; anything from 1 to below it is a server that has.
const maxStratum = 16

// Clock is what the server tells any NTP client about its time, and how that time compares with this machine's.
type Clock struct {
	// Leap is the leap second warning: one of LeapNone, LeapAdd, LeapDrop and LeapNotSet.
	Leap int
	// Version is the NTP version the answer is written in.
	Version int
	// Stratum is how far the server says it is from a reference clock: 1 while it has a GPS fix or is holding over from one, which the manual says
	// it does for 24 hours, and 16 after that.
	Stratum int
	// Poll is how often the server suggests it be asked.
	Poll time.Duration
	// Precision is how fine the server says its clock is, in seconds. The maker's support has said this is what changes in holdover.
	Precision float64
	// RootDelay and RootDispersion are how far the server says it is from its reference. A LeoNTP gives 0 for both, holdover included.
	RootDelay      time.Duration
	RootDispersion time.Duration
	// ReferenceID names what the server takes its time from: "GPS" on a LeoNTP.
	ReferenceID string
	// Reference is when the server says its clock was last set, the zero time when it never has been.
	Reference time.Time
	// Received and Sent are when, by the server's clock, the question reached it and the answer left it.
	Received time.Time
	Sent     time.Time
	// Offset is the server's clock less this machine's, with the journey there and back taken out.
	Offset time.Duration
	// RoundTrip is how long the question and its answer spent on the network.
	RoundTrip time.Duration
}

// Synchronised is whether the server vouches for its time: its clock is set and its stratum says it has a reference.
func (c Clock) Synchronised() bool {
	return c.Leap != LeapNotSet && c.Stratum > 0 && c.Stratum < maxStratum
}

// Clock asks the server the time, as any NTP client would.
func (c *Client) Clock(ctx context.Context) (Clock, error) {
	clock, err := retry(ctx, c.attempts, func() (Clock, error) {
		var question []byte
		answer, sent, received, err := c.ask(ctx, func(now time.Time) []byte {
			question = clockQuestion(now)
			return question
		})
		if err != nil {
			return Clock{}, err
		}

		return parseClock(answer, question, sent, received)
	})
	if err != nil {
		return Clock{}, fmt.Errorf("asking %s the time: %w", c.addr, err)
	}

	return clock, nil
}

// clockQuestion is the ordinary NTP client's question, stamped with the moment it leaves so the answer can be matched to it.
func clockQuestion(now time.Time) []byte {
	q := make([]byte, packetSize)
	q[0] = ntpVersion<<3 | modeClient

	seconds, fraction := ntpTimestamp(now)
	binary.BigEndian.PutUint32(q[40:44], seconds)
	binary.BigEndian.PutUint32(q[44:48], fraction)

	return q
}

// parseClock reads the answer to question, which left at sent and was answered at received by this machine's clock.
func parseClock(b, question []byte, sent, received time.Time) (Clock, error) {
	if len(b) < packetSize {
		return Clock{}, fmt.Errorf("the answer is %d bytes, not the %d of a whole one", len(b), packetSize)
	}

	if b[0]&modeMask != modeServer {
		return Clock{}, fmt.Errorf("the answer is not an NTP server's (it starts 0x%02x)", b[0])
	}

	// a server hands back the stamp it was sent, so an answer without it belongs to some other question
	if !bytes.Equal(b[24:32], question[40:48]) {
		return Clock{}, errors.New("the answer is to a question that was not asked")
	}

	be := binary.BigEndian
	arrived := ntpTime(be.Uint32(b[32:36]), be.Uint32(b[36:40]))
	left := ntpTime(be.Uint32(b[40:44]), be.Uint32(b[44:48]))

	return Clock{
		Leap:           int(b[0] >> 6),
		Version:        int(b[0] >> 3 & 0x07),
		Stratum:        int(b[1]),
		Poll:           time.Duration(powerOfTwo(b[2]) * float64(time.Second)),
		Precision:      powerOfTwo(b[3]),
		RootDelay:      ntpShort(be.Uint32(b[4:8])),
		RootDispersion: ntpShort(be.Uint32(b[8:12])),
		ReferenceID:    strings.TrimRight(string(b[12:16]), "\x00"),
		Reference:      ntpTime(be.Uint32(b[16:20]), be.Uint32(b[20:24])),
		Received:       arrived,
		Sent:           left,
		Offset:         (arrived.Sub(sent) + left.Sub(received)) / 2,
		RoundTrip:      received.Sub(sent) - left.Sub(arrived),
	}, nil
}

// powerOfTwo is 2 to the power b holds, which NTP writes as a signed byte: 6 for 64 seconds, -25 for 30 nanoseconds.
func powerOfTwo(b byte) float64 {
	return math.Ldexp(1, int(int8(b))) //nolint:gosec // G115: the byte is a signed number, and is read as one
}
