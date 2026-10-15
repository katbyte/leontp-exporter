package leontp_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katbyte/leontp-exporter/sdk/leontp"
)

func TestNew(t *testing.T) {
	t.Parallel()

	for server, want := range map[string]string{
		"10.0.0.10":        "10.0.0.10:123",
		"ntp.example":      "ntp.example:123",
		"ntp.example:1123": "ntp.example:1123",
		"fe80::1":          "[fe80::1]:123",
		"[fe80::1]":        "[fe80::1]:123",
		"[fe80::1]:1123":   "[fe80::1]:1123",
	} {
		c, err := leontp.New(server, leontp.Options{})
		if err != nil {
			t.Errorf("New(%q): %v", server, err)
			continue
		}

		if got := c.Addr(); got != want {
			t.Errorf("New(%q).Addr() = %q, want %q", server, got, want)
		}
	}

	if _, err := leontp.New("", leontp.Options{}); err == nil {
		t.Error("New took no server at all")
	}

	if _, err := leontp.New("10.0.0.10", leontp.Options{Timeout: -time.Second}); err == nil {
		t.Error("New took less than no time to wait for an answer")
	}

	if _, err := leontp.New("10.0.0.10", leontp.Options{Attempts: -1}); err == nil {
		t.Error("New took fewer than no times to ask")
	}
}

func TestALostPacketIsAskedForAgain(t *testing.T) {
	t.Parallel()

	var asked atomic.Int32
	f := serve(t, func([]byte) []byte {
		if asked.Add(1) == 1 {
			return nil
		}

		return packet(t, realStats)
	})

	if _, err := f.client(t, leontp.Options{Timeout: 50 * time.Millisecond}).Stats(t.Context()); err != nil {
		t.Errorf("Stats: %v", err)
	}
}

func TestASilentServerIsAskedTwiceThenGivenUpOn(t *testing.T) {
	t.Parallel()

	f := serve(t, func([]byte) []byte { return nil })
	c := f.client(t, leontp.Options{Timeout: 50 * time.Millisecond})

	_, err := c.Stats(t.Context())
	if err == nil || !strings.Contains(err.Error(), "no answer within 50ms") || !strings.Contains(err.Error(), c.Addr()) {
		t.Errorf("Stats error = %v, want one naming the server and saying there was no answer within 50ms", err)
	}

	if n := f.questions.Load(); n != leontp.DefaultAttempts {
		t.Errorf("the server was asked %d times, want %d", n, leontp.DefaultAttempts)
	}
}

func TestAttemptsIsHowManyTimesAQuestionIsAsked(t *testing.T) {
	t.Parallel()

	f := serve(t, func([]byte) []byte { return nil })

	if _, err := f.client(t, leontp.Options{Timeout: 20 * time.Millisecond, Attempts: 4}).Clock(t.Context()); err == nil {
		t.Error("Clock gave no error for a server that never answered")
	}

	if n := f.questions.Load(); n != 4 {
		t.Errorf("the server was asked %d times, want 4", n)
	}
}

func TestAQuestionNobodyWantsAnsweredIsNotWaitedFor(t *testing.T) {
	t.Parallel()

	f := serve(t, func([]byte) []byte { return nil })
	c := f.client(t, leontp.Options{Timeout: time.Minute})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.Stats(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Stats gave no error for a server that never answered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stats was still waiting long after its context ended")
	}

	if n := f.questions.Load(); n != 1 {
		t.Errorf("the server was asked %d times after nobody wanted the answer, want 1", n)
	}
}

func TestAServerThatCannotBeFoundIsNamed(t *testing.T) {
	t.Parallel()

	c, err := leontp.New("no-such-host.invalid", leontp.Options{Timeout: 50 * time.Millisecond, Attempts: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Stats(t.Context()); err == nil || !strings.Contains(err.Error(), "no-such-host.invalid") {
		t.Errorf("Stats error = %v, want one naming the server", err)
	}
}
