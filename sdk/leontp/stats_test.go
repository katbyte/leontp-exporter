package leontp_test

import (
	"math"
	"testing"
	"time"

	"github.com/katbyte/leontp-exporter/sdk/leontp"
)

// The answer a real 1200 gave is read as its front panel showed it at the time.
func TestStats(t *testing.T) {
	t.Parallel()

	got, err := a1200(t).client(t, leontp.Options{}).Stats(t.Context())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	want := leontp.Stats{
		Time:         time.Unix(1791789103, 404391277),
		Uptime:       851353 * time.Second,
		Requests:     156813,
		LockTime:     31992 * time.Second,
		Satellites:   7,
		Flags:        1,
		Serial:       416,
		Firmware:     "2.07",
		FirmwareWord: 0x0207,
	}
	if !got.Time.Equal(want.Time) {
		t.Errorf("Time = %s, want %s", got.Time.UTC(), want.Time.UTC())
	}

	got.Time, want.Time = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("Stats = %+v, want %+v", got, want)
	}
}

// A 1200 stays silent for the 8 bytes the maker's script sends, so that is not what is sent unless it is asked for.
func TestTheQuestionIsTheSizeA1200Answers(t *testing.T) {
	t.Parallel()

	sizes := make(chan int, 2)
	f := serve(t, func(q []byte) []byte {
		sizes <- len(q)
		if !isStatsQuestion(q) {
			return nil
		}

		return packet(t, realStats)
	})

	if _, err := f.client(t, leontp.Options{}).Stats(t.Context()); err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if _, err := f.client(t, leontp.Options{ShortQuestion: true}).Stats(t.Context()); err != nil {
		t.Fatalf("Stats with the short question: %v", err)
	}

	if first, second := <-sizes, <-sizes; first != wholeQuestion || second != shortQuestion {
		t.Errorf("the questions were %d and then %d bytes, want %d and then %d", first, second, wholeQuestion, shortQuestion)
	}
}

// The older unit keeps something above its version, which is left out of the version and kept in the word it came from.
func TestStatsOfAnOlderUnit(t *testing.T) {
	t.Parallel()

	answer := packet(t, realStats)
	// firmware 1.23 as a public unit's page shows it: 4a01.23
	copy(answer[44:48], []byte{0x23, 0x01, 0x4a, 0x00})
	// two ntpq questions answered, which firmware 1.12 of the older unit does
	answer[32] = 2
	f := serve(t, func([]byte) []byte { return answer })

	got, err := f.client(t, leontp.Options{ShortQuestion: true}).Stats(t.Context())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if got.Firmware != "1.23" || got.FirmwareWord != 0x4a0123 || got.ControlRequests != 2 {
		t.Errorf("Firmware, FirmwareWord, ControlRequests = %q, %#x, %d, want 1.23, 0x4a0123 and 2", got.Firmware, got.FirmwareWord, got.ControlRequests)
	}
}

func TestStatsTimeAfter2036(t *testing.T) {
	t.Parallel()

	answer := packet(t, realStats)
	// 16 seconds into NTP's second count of seconds, which starts on 7 February 2036
	copy(answer[16:24], []byte{0, 0, 0, 0, 16, 0, 0, 0})
	f := serve(t, func([]byte) []byte { return answer })

	got, err := f.client(t, leontp.Options{}).Stats(t.Context())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if want := time.Date(2036, time.February, 7, 6, 28, 32, 0, time.UTC); !got.Time.Equal(want) {
		t.Errorf("Time = %s, want %s", got.Time.UTC(), want)
	}
}

func TestStatsRefusesWhatIsNotStatistics(t *testing.T) {
	t.Parallel()

	for name, answer := range map[string][]byte{
		"an answer cut short":     packet(t, realStats)[:40],
		"an ordinary NTP answer":  packet(t, realClock),
		"its own question echoed": append([]byte{0x27, 0, 0x10, 1}, make([]byte, 44)...),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := serve(t, func([]byte) []byte { return answer })

			if got, err := f.client(t, leontp.Options{}).Stats(t.Context()); err == nil {
				t.Errorf("Stats = %+v, want an error", got)
			}
		})
	}
}

func TestAverageRate(t *testing.T) {
	t.Parallel()

	if got := (leontp.Stats{Requests: 1200, Uptime: 10 * time.Minute}).AverageRate(); math.Abs(got-2) > 1e-9 {
		t.Errorf("AverageRate = %g, want 2 a second", got)
	}

	if got := (leontp.Stats{Requests: 5}).AverageRate(); got != 0 {
		t.Errorf("AverageRate of a server only just powered on = %g, want 0", got)
	}
}
