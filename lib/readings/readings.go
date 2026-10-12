// Package readings is what a LeoNTP time server says about itself, as Prometheus readings for a metrics page to hold.
package readings

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/leontp-metrics/sdk/leontp"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace starts every reading's name.
const Namespace = "leontp"

// all is every reading there can be, for Describe.
var all []*prometheus.Desc

// reading describes one reading of the time server.
func reading(name, help string, labels ...string) *prometheus.Desc {
	d := prometheus.NewDesc(Namespace+"_"+name, help, labels, nil)
	all = append(all, d)

	return d
}

var (
	up = reading("up", "Whether the time server answered both questions the last time it was asked (1) or not (0).")

	deviceInfo      = reading("device_info", "Always 1, with the unit's serial number and firmware version.", "serial", "firmware")
	uptime          = reading("uptime_seconds", "How long the time server has been powered on.")
	requests        = reading("ntp_requests_total", "NTP requests the time server has answered since it was powered on.")
	controlRequests = reading("control_requests_total", "ntpq questions the time server has answered since it was powered on; a 1200 answers none.")
	gpsLock         = reading("gps_lock_seconds", "How long the GPS has been locked; it starts again from 0 each time the fix is lost and found.")
	satellites      = reading("gps_satellites", "Satellites the GPS receiver is using.")
	flags           = reading("status_flags", "A status byte the maker does not document, as a number: 1 on a healthy, locked unit.")

	stratum   = reading("stratum", "The stratum the time server gives NTP clients: 1 while it has a GPS fix or is holding over from one.")
	leap      = reading("leap_indicator", "The leap second warning given to NTP clients: 0 none, 1 a second to be added, 2 one to be dropped, 3 clock not set.")
	precision = reading("precision_seconds", "How fine the time server says its clock is.")
	reference = reading("reference_timestamp_seconds", "When the time server says its clock was last set, in seconds since 1970.")
	offset    = reading("clock_offset_seconds", "The time server's clock less the clock of the machine running leontp-metrics, with the network delay taken out.")
	roundTrip = reading("round_trip_seconds", "How long an NTP question and its answer spent on the network.")
)

// Server is what is asked: a LeoNTP, or a test's own.
type Server interface {
	Stats(ctx context.Context) (leontp.Stats, error)
	Clock(ctx context.Context) (leontp.Clock, error)
}

// Readings asks the time server both questions each time the page is read, so a scrape holds what the server said a moment ago and nothing older.
type Readings struct {
	server Server

	mu        sync.Mutex
	asked     bool
	answering bool
}

// New is the readings of this time server.
func New(server Server) *Readings {
	return &Readings{server: server}
}

// Describe names every reading there can be.
func (*Readings) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range all {
		ch <- d
	}
}

// Collect asks both questions at once and writes every reading there is an answer for. A question that goes unanswered leaves its readings off
// the page, which is not the same as readings of zero.
func (r *Readings) Collect(ch chan<- prometheus.Metric) {
	var stats leontp.Stats
	var clock leontp.Clock
	var statsErr, clockErr error

	// a scrape hands a collector no context of its own, and each question gives up by itself
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Go(func() { stats, statsErr = r.server.Stats(ctx) })
	wg.Go(func() { clock, clockErr = r.server.Clock(ctx) })
	wg.Wait()

	// one line in the log whether one question went unanswered or both
	err := cmp.Or(statsErr, clockErr)
	if statsErr != nil && clockErr != nil {
		err = fmt.Errorf("%w; %w", statsErr, clockErr)
	}

	w := writer{ch: ch}
	w.flag(up, r.note(err))

	if statsErr == nil {
		w.gauge(deviceInfo, 1, strconv.Itoa(stats.Serial), stats.Firmware)
		w.gauge(uptime, stats.Uptime.Seconds())
		w.counter(requests, float64(stats.Requests))
		w.counter(controlRequests, float64(stats.ControlRequests))
		w.gauge(gpsLock, stats.LockTime.Seconds())
		w.gauge(satellites, float64(stats.Satellites))
		w.gauge(flags, float64(stats.Flags))
	}

	if clockErr == nil {
		w.gauge(stratum, float64(clock.Stratum))
		w.gauge(leap, float64(clock.Leap))
		w.gauge(precision, clock.Precision)
		w.gauge(offset, clock.Offset.Seconds())
		w.gauge(roundTrip, clock.RoundTrip.Seconds())

		if !clock.Reference.IsZero() {
			w.at(reference, clock.Reference)
		}
	}
}

// note records whether the time server answered, and logs when it stops answering and when it starts again rather than at every scrape in between.
func (r *Readings) note(err error) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	was, asked := r.answering, r.asked
	r.answering, r.asked = err == nil, true

	switch {
	case err != nil && (was || !asked):
		clog.Log.Warnf("the time server is not answering: %v", err)
	case err == nil && asked && !was:
		clog.Log.Info("the time server is answering again")
	}

	return r.answering
}

// Answering is whether the time server answered both questions the last time it was asked.
func (r *Readings) Answering() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.answering
}

// writer writes the time server's readings.
type writer struct {
	ch chan<- prometheus.Metric
}

// gauge writes one reading that can go up and down.
func (w writer) gauge(d *prometheus.Desc, value float64, labels ...string) {
	w.ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, value, labels...)
}

// counter writes one reading that only goes up, until the time server restarts.
func (w writer) counter(d *prometheus.Desc, value float64) {
	w.ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, value)
}

// flag is gauge for a yes or no: 1 or 0.
func (w writer) flag(d *prometheus.Desc, yes bool) {
	if yes {
		w.gauge(d, 1)
		return
	}

	w.gauge(d, 0)
}

// at is gauge for a moment, in seconds since 1970.
func (w writer) at(d *prometheus.Desc, t time.Time) {
	w.gauge(d, float64(t.UnixMilli())/1000)
}
