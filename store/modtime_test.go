package store

import (
	"testing"
	"time"
)

func TestDecodeExactModTimeRequiresLegacyPrecisionAgreement(t *testing.T) {
	exact := time.Unix(1_700_000_000, 123456789)
	ns, ok := exactModTimeNanos(exact)
	if !ok {
		t.Fatal("expected timestamp to fit Unix nanoseconds")
	}
	got, trusted := decodeExactModTime(exact.Truncate(time.Microsecond), &ns)
	if !trusted || !got.Equal(exact) {
		t.Fatalf("matching timestamp = %v trusted=%v", got, trusted)
	}
	legacy := exact.Add(time.Second).Truncate(time.Microsecond)
	got, trusted = decodeExactModTime(legacy, &ns)
	if trusted || !got.Equal(legacy) {
		t.Fatalf("inconsistent legacy timestamp = %v trusted=%v", got, trusted)
	}
}

func TestExactModTimeNanosRejectsOutOfRange(t *testing.T) {
	if _, ok := exactModTimeNanos(time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("out-of-range timestamp trusted")
	}
}

func TestPostgresCompatibilityModTimeNormalizesOffsetToUTC(t *testing.T) {
	local := time.Date(2023, 11, 14, 16, 13, 20, 123456789, time.FixedZone("CST", -6*60*60))
	want := time.Date(2023, 11, 14, 22, 13, 20, 123456000, time.UTC)
	got := postgresCompatibilityModTime(local)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("compatibility timestamp = %v, want %v", got, want)
	}
	ns, _ := exactModTimeNanos(local)
	decoded, trusted := decodeExactModTime(got, &ns)
	if !trusted || !decoded.Equal(local) {
		t.Fatalf("decoded timestamp = %v trusted=%v", decoded, trusted)
	}
}
