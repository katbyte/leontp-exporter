package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katbyte/go-kt/metrics/page"
	"github.com/katbyte/leontp-metrics/lib/readings"
	"github.com/katbyte/leontp-metrics/sdk/leontp"
	"github.com/spf13/cobra"
)

// box is a made-up time server: seven satellites, stratum 1, and silent while down is set. It counts how often its statistics are asked for.
type box struct {
	down  atomic.Bool
	asked atomic.Int32
}

func (b *box) Stats(context.Context) (leontp.Stats, error) {
	b.asked.Add(1)
	if b.down.Load() {
		return leontp.Stats{}, errors.New("no answer within 1s")
	}

	return leontp.Stats{Satellites: 7, Serial: 416, Firmware: "2.07"}, nil
}

func (b *box) Clock(context.Context) (leontp.Clock, error) {
	if b.down.Load() {
		return leontp.Clock{}, errors.New("no answer within 1s")
	}

	return leontp.Clock{Stratum: 1}, nil
}

// asking is a connector that hands out this time server whatever address it is given.
func asking(b *box) connector {
	return func(string, leontp.Options) (readings.Server, error) { return b, nil }
}

// program is the program given these arguments and this time server, not yet run, and where what it prints will go.
func program(t *testing.T, connect connector, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	root, err := makeWith(connect)
	if err != nil {
		t.Fatalf("makeWith: %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	return root, &out
}

// run executes the program with these arguments against this time server, and gives back what it printed.
func run(t *testing.T, connect connector, args ...string) (string, error) {
	t.Helper()

	root, out := program(t, connect, args...)
	err := root.ExecuteContext(t.Context())

	return out.String(), err
}

//nolint:paralleltest // viper is global state; Make mutates it
func TestMakeBuildsTheCommands(t *testing.T) {
	alone(t)

	root, err := Make()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"version", "print", "serve"} {
		if c, _, err := root.Find([]string{name}); err != nil || c.Name() != name {
			t.Errorf("no %s command: %v", name, err)
		}
	}
	// a setting is a flag on every command, so it can be given after whichever one is run
	for _, name := range []string{"server", "listen", "labels", "timeout", "short-question", "config"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("no --%s flag", name)
		}
	}
}

// The real connector makes a client without asking the time server anything, so a wrong address is found by print and not by starting.
func TestConnect(t *testing.T) {
	t.Parallel()

	if _, err := connect(testServer, leontp.Options{}); err != nil {
		t.Errorf("connect: %v", err)
	}

	if _, err := connect("", leontp.Options{}); err == nil {
		t.Error("connect took no server at all")
	}
}

//nolint:paralleltest // viper is global state; Make mutates it
func TestVersionCommand(t *testing.T) {
	alone(t)

	out, err := run(t, asking(&box{}), "version")
	if err != nil || !strings.HasPrefix(out, "leontp-metrics ") {
		t.Errorf("version printed %q (%v), want the name and a version", out, err)
	}
}

// With no command the program says where the help is rather than serving or failing.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestNoCommandPointsAtHelp(t *testing.T) {
	alone(t)

	out, err := run(t, asking(&box{}), "--server", testServer)
	if err != nil || !strings.Contains(out, "leontp-metrics help") {
		t.Errorf("no command printed %q (%v), want a pointer to help", out, err)
	}
}

// print asks once and writes the page a scrape would read, with the labels asked for on every reading.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintWritesThePage(t *testing.T) {
	alone(t)

	b := &box{}
	out, err := run(t, asking(b), "print", "--server", testServer, "--labels", "hostname=ntp,job=leontp")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`leontp_up{hostname="ntp",job="leontp"} 1`,
		`leontp_gps_satellites{hostname="ntp",job="leontp"} 7`,
		`leontp_stratum{hostname="ntp",job="leontp"} 1`,
		`leontp_device_info{firmware="2.07",hostname="ntp",job="leontp",serial="416"} 1`,
		`leontp_metrics_build_info{`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the page lacks %s:\n%s", want, out)
		}
	}

	if n := b.asked.Load(); n != 1 {
		t.Errorf("print asked the time server %d times, want once", n)
	}
}

// print is also the check that an address works, so a server that does not answer makes it fail, with the page still printed to show what that
// looks like.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintFailsWhenTheServerDoesNotAnswer(t *testing.T) {
	alone(t)

	b := &box{}
	b.down.Store(true)

	out, err := run(t, asking(b), "print", "--server", testServer)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("print with the server down = %v, want an error saying it did not answer", err)
	}

	if !strings.Contains(out, "leontp_up 0\n") || strings.Contains(out, "leontp_stratum") {
		t.Errorf("the page should say the server is down and hold none of its readings:\n%s", out)
	}
}

// With no server named there is nothing to report on, and both commands say which variable names one rather than serve an empty page. Labels
// that cannot be put on the page stop both the same way.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestACommandNeedsAServerAndLabelsItCanUse(t *testing.T) {
	for _, command := range []string{"print", "serve"} {
		alone(t)

		if _, err := run(t, asking(&box{}), command); err == nil || !strings.Contains(err.Error(), "LEONTP_METRICS_SERVER") {
			t.Errorf("%s with no server = %v, want an error naming the variable to set", command, err)
		}

		args := []string{command, "--server", testServer, "--listen", "127.0.0.1:0", "--labels", "serial=1"}
		if _, err := run(t, asking(&box{}), args...); err == nil || !strings.Contains(err.Error(), "LEONTP_METRICS_LABELS") {
			t.Errorf("%s with a label a reading already has = %v, want an error naming the variable", command, err)
		}
	}
}

// freePort is an address on this machine nothing else is listening on: taken, noted and given back.
func freePort(t *testing.T) string {
	t.Helper()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	return ln.Addr().String()
}

// fetch reads a path from the program once it is serving, trying for a second while it starts.
func fetch(t *testing.T, addr, path string) string {
	t.Helper()

	for range 50 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		return string(body)
	}
	t.Fatalf("nothing answered at %s%s", addr, path)

	return ""
}

// serve is the whole program: it serves what the time server says each time the page is read, says when the server stops answering, and stops
// cleanly when told to.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestServe(t *testing.T) {
	alone(t)

	b := &box{}
	addr := freePort(t)

	root, _ := program(t, asking(b), "serve", "--listen", addr, "--server", testServer, "--labels", "hostname=ntp")
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- root.ExecuteContext(ctx) }()

	if got := strings.TrimSpace(fetch(t, addr, page.HealthPath)); got != "ok" {
		t.Errorf("%s = %q, want ok", page.HealthPath, got)
	}

	if got := fetch(t, addr, "/"); !strings.HasPrefix(got, "leontp-metrics ") {
		t.Errorf("the root = %q, want the program's name and version", got)
	}

	body := fetch(t, addr, page.MetricsPath)
	for _, want := range []string{`leontp_up{hostname="ntp"} 1`, `leontp_gps_satellites{hostname="ntp"} 7`, `leontp_stratum{hostname="ntp"} 1`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %s:\n%s", want, body)
		}
	}

	// the server is asked at each read of the page, so the next one says it has stopped answering
	b.down.Store(true)
	if body := fetch(t, addr, page.MetricsPath); !strings.Contains(body, `leontp_up{hostname="ntp"} 0`) || strings.Contains(body, "leontp_stratum") {
		t.Errorf("the page should say the server is down and hold none of its readings:\n%s", body)
	}

	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("serve stopped with %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop when told to")
	}
}

// A port that is already taken stops the program with the address in the error.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestServeSaysWhenThePortIsTaken(t *testing.T) {
	alone(t)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	root, _ := program(t, asking(&box{}), "serve", "--listen", addr, "--server", testServer)
	stopped := make(chan error, 1)
	go func() { stopped <- root.ExecuteContext(t.Context()) }()

	select {
	case err := <-stopped:
		if err == nil || !strings.Contains(err.Error(), addr) {
			t.Errorf("serve on a taken port = %v, want an error naming %s", err, addr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not give up on a port that was taken")
	}
}
