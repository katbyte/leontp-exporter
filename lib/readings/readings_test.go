package readings_test

import (
	"context"
	"errors"
	"flag"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/go-kt/pointer"
	"github.com/katbyte/go-kt/prometheus/metrics"
	"github.com/katbyte/leontp-exporter/lib/readings"
	"github.com/katbyte/leontp-exporter/sdk/leontp"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
)

// saved is where the page is kept as it was last agreed.
const saved = "testdata/page.txt"

// update rewrites the saved page from what the code writes now: go test ./lib/readings -update. Read the difference before keeping it, since a
// reading that is renamed is one somebody's dashboard no longer finds.
var update = flag.Bool("update", false, "rewrite testdata/page.txt from the page as it is written now")

// fixed is a time server that answers whatever the test says, or not at all when down is set.
type fixed struct {
	stats leontp.Stats
	clock leontp.Clock
	down  error
}

func (f *fixed) Stats(context.Context) (leontp.Stats, error) { return f.stats, f.down }
func (f *fixed) Clock(context.Context) (leontp.Clock, error) { return f.clock, f.down }

// locked is a healthy 1200: up nearly ten days, locked to GPS for the last nine hours, with a clock 87 milliseconds from this machine's.
func locked() *fixed {
	return &fixed{
		stats: leontp.Stats{Uptime: 851353 * time.Second, Requests: 156813, LockTime: 31992 * time.Second, Satellites: 7, Flags: 1, Serial: 416, Firmware: "2.07"},
		clock: leontp.Clock{Stratum: 1, Precision: math.Ldexp(1, -25), Reference: time.Unix(1791789057, 0), Offset: 87320658 * time.Nanosecond, RoundTrip: 13565941 * time.Nanosecond},
	}
}

// statsOnly is a server that gives its statistics and will not say the time.
type statsOnly struct{ *fixed }

func (statsOnly) Clock(context.Context) (leontp.Clock, error) {
	return leontp.Clock{}, errors.New("no answer within 1s")
}

// whole is the page of r as text, the reading that says which Go built it included.
func whole(t *testing.T, r *readings.Readings, labels map[string]string) string {
	t.Helper()

	p, err := metrics.New(metrics.Options{Name: "leontp-exporter", Labels: labels}, r)
	if err != nil {
		t.Fatalf("metrics.New: %v", err)
	}

	var b strings.Builder
	if err := p.Write(&b); err != nil {
		t.Fatalf("Write: %v", err)
	}

	return b.String()
}

// written is the page of r as text, less the one reading that says which Go built it.
func written(t *testing.T, r *readings.Readings) string {
	t.Helper()

	var lines []string
	for line := range strings.Lines(whole(t, r, nil)) {
		if !strings.HasPrefix(line, "leontp_exporter_build_info{") {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "")
}

func TestThePageIsWhatWasSaved(t *testing.T) {
	t.Parallel()

	got := written(t, readings.New(locked()))

	if pointer.From(update) {
		if err := os.WriteFile(saved, []byte(got), 0o600); err != nil {
			t.Fatalf("saving the page: %v", err)
		}
	}

	want, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("reading the saved page (go test ./lib/readings -update writes it): %v", err)
	}

	if got != string(want) {
		t.Errorf("the page is not the one in %s; if the change is meant, go test ./lib/readings -update\n%s", saved, got)
	}
}

func TestThePageFollowsTheNamingConventions(t *testing.T) {
	t.Parallel()

	problems, err := promlint.New(strings.NewReader(whole(t, readings.New(locked()), nil))).Lint()
	if err != nil {
		t.Fatalf("checking the page: %v", err)
	}

	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}
}

// No label whoever runs the program is likely to want is one a reading already has, so the page can still be made with them on it.
func TestTheLabelsAnyoneWouldSetCanBeSet(t *testing.T) {
	t.Parallel()

	got := whole(t, readings.New(locked()), map[string]string{"hostname": "ntp", "host": "ntp", "job": "leontp", "instance": "ntp", "site": "home"})

	for line := range strings.Lines(got) {
		if !strings.HasPrefix(line, "#") && !strings.Contains(line, `hostname="ntp"`) {
			t.Errorf("a reading without the labels: %s", line)
		}
	}

	if want := `leontp_device_info{firmware="2.07",host="ntp",hostname="ntp",instance="ntp",job="leontp",serial="416",site="home"} 1`; !strings.Contains(got, want) {
		t.Errorf("the page lacks %s:\n%s", want, got)
	}
}

func TestAServerThatDoesNotAnswerHasNoReadingsButThatItIsDown(t *testing.T) {
	t.Parallel()

	r := readings.New(&fixed{down: errors.New("no answer within 1s")})
	got := written(t, r)

	if !strings.Contains(got, "\nleontp_up 0\n") {
		t.Errorf("the page does not say the server is down:\n%s", got)
	}

	for line := range strings.Lines(got) {
		if !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "leontp_up ") {
			t.Errorf("the page holds a reading of a server that did not answer: %s", line)
		}
	}

	if r.Answering() {
		t.Error("Answering is true of a server that did not answer")
	}
}

func TestWhatWasAnsweredIsKeptWhenOneQuestionIsNot(t *testing.T) {
	t.Parallel()

	got := written(t, readings.New(statsOnly{locked()}))

	if !strings.Contains(got, "\nleontp_up 0\n") || !strings.Contains(got, "\nleontp_gps_satellites 7\n") || strings.Contains(got, "\nleontp_stratum ") {
		t.Errorf("want the server down, its satellites on the page and its stratum off it:\n%s", got)
	}
}

// A server whose clock was never set has no moment it was last set, which is left off rather than written as 1970.
func TestAClockNeverSetHasNoReferenceTime(t *testing.T) {
	t.Parallel()

	server := locked()
	server.clock = leontp.Clock{Stratum: 16, Leap: leontp.LeapNotSet}
	got := written(t, readings.New(server))

	if strings.Contains(got, "\nleontp_reference_timestamp_seconds ") || !strings.Contains(got, "\nleontp_stratum 16\n") || !strings.Contains(got, "\nleontp_leap_indicator 3\n") {
		t.Errorf("want stratum 16, leap 3 and no reference time:\n%s", got)
	}
}

// Answering follows the server: down, then back.
func TestAnsweringFollowsTheServer(t *testing.T) {
	t.Parallel()

	server := locked()
	r := readings.New(server)

	if r.Answering() {
		t.Error("Answering is true before the server was ever asked")
	}

	written(t, r)
	if !r.Answering() {
		t.Error("Answering is false of a server that answered")
	}

	server.down = errors.New("no answer within 1s")
	written(t, r)
	if r.Answering() {
		t.Error("Answering is true of a server that stopped answering")
	}

	server.down = nil
	written(t, r)
	if !r.Answering() {
		t.Error("Answering is false of a server that came back")
	}
}
