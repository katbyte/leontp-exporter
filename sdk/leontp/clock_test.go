package leontp_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/leontp-exporter/sdk/leontp"
)

// The ordinary answer a real 1200 gave, with its clock a quarter of a second ahead of this machine's.
func TestClock(t *testing.T) {
	t.Parallel()

	const ahead = 250 * time.Millisecond

	f := serve(t, func(q []byte) []byte {
		if len(q) != wholeQuestion || q[0] != 0x23 {
			return nil
		}

		return answerTo(q, packet(t, realClock), ahead)
	})

	got, err := f.client(t, leontp.Options{}).Clock(t.Context())
	if err != nil {
		t.Fatalf("Clock: %v", err)
	}

	if got.Leap != leontp.LeapNone || got.Version != 4 || got.Stratum != 1 || got.ReferenceID != "GPS" {
		t.Errorf("Leap, Version, Stratum, ReferenceID = %d, %d, %d, %q, want 0, 4, 1 and GPS", got.Leap, got.Version, got.Stratum, got.ReferenceID)
	}

	if got.Poll != 64*time.Second || got.Precision != math.Ldexp(1, -25) {
		t.Errorf("Poll, Precision = %s, %g, want 1m4s and %g", got.Poll, got.Precision, math.Ldexp(1, -25))
	}

	if got.RootDelay != 0 || got.RootDispersion != 0 {
		t.Errorf("RootDelay, RootDispersion = %s, %s, want nothing for either", got.RootDelay, got.RootDispersion)
	}

	if want := time.Unix(1791789057, 0); !got.Reference.Equal(want) {
		t.Errorf("Reference = %s, want %s", got.Reference.UTC(), want.UTC())
	}

	if diff := got.Offset - ahead; diff < -50*time.Millisecond || diff > 50*time.Millisecond {
		t.Errorf("Offset = %s, want about %s", got.Offset, ahead)
	}

	if got.RoundTrip < 0 || got.RoundTrip > time.Second {
		t.Errorf("RoundTrip = %s, want a moment", got.RoundTrip)
	}

	if got.Sent.Before(got.Received) || time.Until(got.Received) > time.Second {
		t.Errorf("Received, Sent = %s, %s, want the server's clock as the question came and went", got.Received, got.Sent)
	}

	if !got.Synchronised() {
		t.Error("a locked server is not taken to vouch for its time")
	}
}

// A server further from its reference says how far, as a span NTP writes in 16 bits each side of the point.
func TestClockOfAServerWithAReferenceFurtherOff(t *testing.T) {
	t.Parallel()

	f := serve(t, func(q []byte) []byte {
		a := answerTo(q, packet(t, realClock), 0)
		a[1] = 2
		copy(a[4:12], []byte{0x00, 0x00, 0x80, 0x00, 0x00, 0x01, 0x40, 0x00})

		return a
	})

	got, err := f.client(t, leontp.Options{}).Clock(t.Context())
	if err != nil {
		t.Fatalf("Clock: %v", err)
	}

	if got.RootDelay != 500*time.Millisecond || got.RootDispersion != 1250*time.Millisecond || got.Stratum != 2 {
		t.Errorf("RootDelay, RootDispersion, Stratum = %s, %s, %d, want 500ms, 1.25s and 2", got.RootDelay, got.RootDispersion, got.Stratum)
	}
}

// The manual says a unit a day without GPS gives stratum 16. Such a server, or one whose clock was never set, does not vouch for its time.
func TestClockOfAServerNeverSet(t *testing.T) {
	t.Parallel()

	f := serve(t, func(q []byte) []byte {
		a := answerTo(q, packet(t, realClock), 0)
		a[0], a[1] = 0xe4, 16
		clear(a[16:24])

		return a
	})

	got, err := f.client(t, leontp.Options{}).Clock(t.Context())
	if err != nil {
		t.Fatalf("Clock: %v", err)
	}

	if got.Stratum != 16 || got.Leap != leontp.LeapNotSet || !got.Reference.IsZero() {
		t.Errorf("Stratum, Leap, Reference = %d, %d, %s, want 16, 3 and no time", got.Stratum, got.Leap, got.Reference)
	}

	if got.Synchronised() {
		t.Error("a server whose clock is not set is taken to vouch for its time")
	}
}

func TestSynchronised(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		clock leontp.Clock
		want  bool
	}{
		"locked":                    {leontp.Clock{Stratum: 1}, true},
		"a leap second coming":      {leontp.Clock{Stratum: 1, Leap: leontp.LeapAdd}, true},
		"stratum 16":                {leontp.Clock{Stratum: 16}, false},
		"stratum 0":                 {leontp.Clock{Stratum: 0}, false},
		"stratum 1, clock not set":  {leontp.Clock{Stratum: 1, Leap: leontp.LeapNotSet}, false},
		"stratum 15, the last good": {leontp.Clock{Stratum: 15, Leap: leontp.LeapDrop}, true},
	} {
		if got := c.clock.Synchronised(); got != c.want {
			t.Errorf("%s: Synchronised = %v, want %v", name, got, c.want)
		}
	}
}

func TestClockRefusesWhatIsNotAnAnswerToItsQuestion(t *testing.T) {
	t.Parallel()

	for name, c := range map[string]struct {
		answer func(q []byte) []byte
		want   string
	}{
		// the captured answer carries the stamp of the question it was captured with, not this one's
		"an answer to another question": {func([]byte) []byte { return packet(t, realClock) }, "not asked"},
		"an answer cut short":           {func(q []byte) []byte { return answerTo(q, packet(t, realClock), 0)[:40] }, "40 bytes"},
		"a statistics answer":           {func([]byte) []byte { return packet(t, realStats) }, "not an NTP server's"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := serve(t, c.answer).client(t, leontp.Options{}).Clock(t.Context())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Clock error = %v, want one saying %q", err, c.want)
			}
		})
	}
}
