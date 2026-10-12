package leontp

import "time"

// ntpToUnix is the seconds from 1900, where NTP counts from, to 1970.
const ntpToUnix = 2208988800

// ntpTime is an NTP timestamp as a time, the zero time for a stamp that is all zeros. NTP's count of seconds runs out in 2036 and starts again, so a
// count in its lower half is taken to be after that.
func ntpTime(seconds, fraction uint32) time.Time {
	if seconds == 0 && fraction == 0 {
		return time.Time{}
	}

	s := int64(seconds) - ntpToUnix
	if seconds&0x80000000 == 0 {
		s += 1 << 32
	}

	return time.Unix(s, int64(uint64(fraction)*1e9>>32))
}

// ntpTimestamp is a time as an NTP timestamp.
func ntpTimestamp(t time.Time) (seconds, fraction uint32) {
	return uint32(t.Unix() + ntpToUnix), uint32(uint64(t.Nanosecond()) << 32 / 1e9) //nolint:gosec // G115: NTP's count of seconds is 32 bits and wraps by design
}

// ntpShort is one of NTP's short spans as a duration: whole seconds in the upper half, the fraction of one in the lower.
func ntpShort(v uint32) time.Duration {
	return time.Duration(uint64(v) * uint64(time.Second) >> 16)
}
