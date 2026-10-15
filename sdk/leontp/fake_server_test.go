package leontp_test

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katbyte/leontp-exporter/sdk/leontp"
)

// What a LeoNTP 1200 on firmware 2.07 sent back when asked, byte for byte: the captures in api-defs.
const (
	realStats = "a7 00 06 e7 00 00 00 00 00 00 00 00 47 50 53 00 d0 2f 86 67 af 06 77 ee 99 fd 0c 00 8d 64 02 00 00 00 00 00 f8 7c 00 00 01 07 a0 01 07 02 00 00"
	realClock = "24 01 06 e7 00 00 00 00 00 00 00 00 47 50 53 00 ee 77 06 81 00 00 00 00 ee 77 06 81 9f 0d 50 00 ee 77 06 81 b7 24 7c e9 ee 77 06 81 b7 24 c1 01"
)

const (
	// ntpToUnix is the seconds from 1900, where NTP counts from, to 1970.
	ntpToUnix = 2208988800
	// wholeQuestion is the size a 1200 wants a question to be, and shortQuestion the size the maker's script sends.
	wholeQuestion = 48
	shortQuestion = 8
)

// packet is a captured packet as bytes.
func packet(t *testing.T, capture string) []byte {
	t.Helper()

	b, err := hex.DecodeString(strings.ReplaceAll(capture, " ", ""))
	if err != nil {
		t.Fatalf("reading the captured packet: %v", err)
	}

	return b
}

// fake is a made-up time server on this machine: answer is handed each question and what it returns is sent back, nothing when that is nil.
type fake struct {
	conn      net.PacketConn
	questions atomic.Int32
}

func serve(t *testing.T, answer func(question []byte) []byte) *fake {
	t.Helper()

	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting the made-up time server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	f := &fake{conn: conn}
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			f.questions.Add(1)

			if a := answer(append([]byte(nil), buf[:n]...)); a != nil {
				_, _ = conn.WriteTo(a, from)
			}
		}
	}()

	return f
}

// a1200 is a made-up 1200: it answers the statistics question and the ordinary one, each only when it is exactly 48 bytes, as the real one does.
func a1200(t *testing.T) *fake {
	t.Helper()

	stats, clock := packet(t, realStats), packet(t, realClock)

	return serve(t, func(q []byte) []byte {
		switch {
		case len(q) != wholeQuestion:
			return nil
		case isStatsQuestion(q):
			return stats
		case q[0] == 0x23:
			return answerTo(q, clock, 0)
		default:
			return nil
		}
	})
}

// client is a Client that asks f.
func (f *fake) client(t *testing.T, o leontp.Options) *leontp.Client {
	t.Helper()

	c, err := leontp.New(f.conn.LocalAddr().String(), o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return c
}

// isStatsQuestion is whether q starts as the statistics question does: the private mode, the maker's implementation, request 1.
func isStatsQuestion(q []byte) bool {
	return len(q) >= shortQuestion && q[0] == 0x27 && q[1] == 0 && q[2] == 0x10 && q[3] == 1
}

// answerTo is the captured ordinary answer made into one for q: q's stamp handed back, and the server's clock ahead of this machine's by ahead.
func answerTo(q, captured []byte, ahead time.Duration) []byte {
	a := append([]byte(nil), captured...)
	copy(a[24:32], q[40:48])
	stamp(a[32:40], time.Now().Add(ahead))
	stamp(a[40:48], time.Now().Add(ahead))

	return a
}

// stamp writes t into b as an NTP timestamp.
func stamp(b []byte, t time.Time) {
	binary.BigEndian.PutUint32(b[0:4], uint32(t.Unix()+ntpToUnix))             //nolint:gosec // G115: NTP's count of seconds is 32 bits and wraps by design
	binary.BigEndian.PutUint32(b[4:8], uint32(uint64(t.Nanosecond())<<32/1e9)) //nolint:gosec // G115: a fraction of a second always fits
}
