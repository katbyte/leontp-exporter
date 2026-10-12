// Package leontp asks a LeoNTP time server about itself over its NTP port, the only one it listens on: the maker's own question for its statistics,
// and the ordinary one every NTP client asks. It needs nothing but the standard library, and only ever asks.
//
// A new question the maker adds goes in a file of its own beside stats.go: a request code, what the answer holds, and a method that asks it through
// private.
package leontp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	// DefaultPort is the NTP port, used when a server is named without one.
	DefaultPort = "123"
	// DefaultTimeout is how long a question waits for its answer: long for a box on the same network, short enough to ask twice inside one scrape.
	DefaultTimeout = time.Second
	// DefaultAttempts is how many times a question is asked before the server is taken to be silent: one lost packet is not a server that is down.
	DefaultAttempts = 2
)

const (
	// packetSize is the size of an ordinary NTP packet, and of every answer.
	packetSize = 48
	// shortQuestionSize is the size of the statistics question as the maker's script sends it.
	shortQuestionSize = 8

	ntpVersion  = 4
	modeMask    = 0x07
	modeClient  = 3
	modeServer  = 4
	modePrivate = 7
	responseBit = 0x80

	// leoImplementation marks a private question as one of the maker's own.
	leoImplementation = 0x10
)

// Options says how to ask. The zero value asks as a 1200 wants to be asked.
type Options struct {
	// Timeout is how long one question waits for its answer. Zero is DefaultTimeout.
	Timeout time.Duration
	// Attempts is how many times a question is asked before giving up. Zero is DefaultAttempts.
	Attempts int
	// ShortQuestion asks for statistics in the 8 bytes the maker's script sends, for the older LeoNTP the script was written for. A 1200 answers
	// only the 48 byte question, which is what is sent without this.
	ShortQuestion bool
}

// Client asks one LeoNTP. It keeps nothing between questions, so one Client can be used from many places at once.
type Client struct {
	addr     string
	timeout  time.Duration
	attempts int
	short    bool
}

// New is a client for the LeoNTP at server, a name or an address with or without a port.
func New(server string, o Options) (*Client, error) {
	if server == "" {
		return nil, errors.New("no time server to ask")
	}

	if o.Timeout < 0 || o.Attempts < 0 {
		return nil, fmt.Errorf("the time to wait for an answer and the times to ask cannot be less than nothing: %s and %d", o.Timeout, o.Attempts)
	}

	c := &Client{addr: server, timeout: o.Timeout, attempts: o.Attempts, short: o.ShortQuestion}
	if _, _, err := net.SplitHostPort(server); err != nil {
		c.addr = net.JoinHostPort(strings.Trim(server, "[]"), DefaultPort)
	}

	if c.timeout == 0 {
		c.timeout = DefaultTimeout
	}

	if c.attempts == 0 {
		c.attempts = DefaultAttempts
	}

	return c, nil
}

// Addr is the address the questions are sent to, port included.
func (c *Client) Addr() string {
	return c.addr
}

// private asks one of the maker's own questions, the one with this request code, and gives the answer once it is known to be an answer to a private
// question and whole. What the answer holds is for the caller to read.
func (c *Client) private(ctx context.Context, request byte) ([]byte, error) {
	size := packetSize
	if c.short {
		size = shortQuestionSize
	}

	question := make([]byte, size)
	question[0] = ntpVersion<<3 | modePrivate
	question[2] = leoImplementation
	question[3] = request

	return retry(ctx, c.attempts, func() ([]byte, error) {
		answer, _, _, err := c.ask(ctx, func(time.Time) []byte { return question })
		if err != nil {
			return nil, err
		}

		if len(answer) < packetSize {
			return nil, fmt.Errorf("the answer is %d bytes, not the %d of a whole one", len(answer), packetSize)
		}

		if answer[0]&responseBit == 0 || answer[0]&modeMask != modePrivate {
			return nil, fmt.Errorf("the answer is not one to a private question (it starts 0x%02x)", answer[0])
		}

		return answer, nil
	})
}

// retry asks up to attempts times and gives the last failure when no attempt is answered.
func retry[T any](ctx context.Context, attempts int, ask func() (T, error)) (T, error) {
	var answer T
	var err error
	for range attempts {
		if answer, err = ask(); err == nil || ctx.Err() != nil {
			break
		}
	}

	return answer, err
}

// ask sends one question and waits for one answer, saying when the question left and when the answer came. The question is made at the moment it
// leaves, since the ordinary one carries that moment inside it.
func (c *Client) ask(ctx context.Context, question func(now time.Time) []byte) (answer []byte, sent, received time.Time, err error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", c.addr)
	if err != nil {
		return nil, sent, received, err
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, sent, received, err
	}

	// stop waiting as soon as whoever asked no longer wants the answer
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	sent = time.Now()
	if _, err := conn.Write(question(sent)); err != nil {
		return nil, sent, received, err
	}

	buf := make([]byte, 4*packetSize)
	n, err := conn.Read(buf)
	received = time.Now()

	if err != nil && ctx.Err() != nil {
		return nil, sent, received, ctx.Err()
	}

	if errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, sent, received, fmt.Errorf("no answer within %s", c.timeout)
	}

	if err != nil {
		return nil, sent, received, err
	}

	return buf[:n], sent, received, nil
}
