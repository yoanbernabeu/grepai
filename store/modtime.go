package store

import "time"

func exactModTimeNanos(t time.Time) (int64, bool) {
	const minSecond, maxSecond = int64(-9223372037), int64(9223372036)
	if t.Unix() < minSecond || t.Unix() > maxSecond {
		return 0, false
	}
	ns := t.UnixNano()
	return ns, time.Unix(0, ns).Equal(t)
}

func decodeExactModTime(legacy time.Time, nanos *int64) (time.Time, bool) {
	if nanos == nil {
		return legacy, false
	}
	exact := time.Unix(0, *nanos)
	if !legacy.Truncate(time.Microsecond).Equal(exact.Truncate(time.Microsecond)) {
		return legacy, false
	}
	return exact, true
}

func postgresCompatibilityModTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }
