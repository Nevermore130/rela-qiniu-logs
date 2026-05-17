package qiniu

import (
	"errors"
	"testing"
	"time"
)

func tm(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	return t
}

// resolver: keys containing "good" resolve to their embedded time; others fail.
func fakeResolver(key string, put time.Time) (time.Time, error) {
	switch key {
	case "good-early":
		return tm("2026-05-10 00:00:00"), nil
	case "good-late":
		return tm("2026-05-16 12:00:00"), nil
	case "bad":
		return time.Time{}, errors.New("no match")
	}
	return put, nil
}

func entry(key string) rawEntry {
	return rawEntry{Key: key, Size: 1, PutTime: tm("2026-05-16 09:00:00")}
}

func TestSelectFilesNoFilterIncludesAll(t *testing.T) {
	in := []rawEntry{entry("good-early"), entry("bad"), entry("good-late")}
	out := selectFiles(in, fakeResolver, ListOptions{})
	if len(out) != 3 {
		t.Fatalf("got %d, want 3 (no filter includes all)", len(out))
	}
	for _, f := range out {
		if f.Key == "bad" && !f.LogTime.Equal(tm("2026-05-16 09:00:00")) {
			t.Fatalf("bad LogTime = %v, want PutTime fallback", f.LogTime)
		}
	}
}

func TestSelectFilesFilterExcludesUnresolvedAndOutOfRange(t *testing.T) {
	in := []rawEntry{entry("good-early"), entry("bad"), entry("good-late")}
	opts := ListOptions{From: tm("2026-05-15 00:00:00")}
	out := selectFiles(in, fakeResolver, opts)
	if len(out) != 1 || out[0].Key != "good-late" {
		t.Fatalf("got %+v, want only good-late", out)
	}
}

func TestSelectFilesRespectsLimit(t *testing.T) {
	in := []rawEntry{entry("good-early"), entry("good-late"), entry("other")}
	out := selectFiles(in, fakeResolver, ListOptions{Limit: 2})
	if len(out) != 2 {
		t.Fatalf("got %d, want 2 (limit)", len(out))
	}
}

func TestSelectFilesToBoundAndExclusionReasons(t *testing.T) {
	// To-bound: only good-early (2026-05-10) is <= To; good-late excluded as out-of-range; bad excluded as unresolved.
	in := []rawEntry{entry("good-early"), entry("bad"), entry("good-late")}
	out := selectFiles(in, fakeResolver, ListOptions{To: tm("2026-05-11 00:00:00")})
	if len(out) != 1 || out[0].Key != "good-early" {
		t.Fatalf("To-bound: got %+v, want only good-early", out)
	}

	// Unresolved-only with an active filter is excluded.
	out = selectFiles([]rawEntry{entry("bad")}, fakeResolver, ListOptions{From: tm("2026-05-01 00:00:00")})
	if len(out) != 0 {
		t.Fatalf("unresolved+filter: got %+v, want none", out)
	}

	// Resolved-but-out-of-range is excluded (distinct from unresolved).
	out = selectFiles([]rawEntry{entry("good-early")}, fakeResolver, ListOptions{From: tm("2026-05-15 00:00:00")})
	if len(out) != 0 {
		t.Fatalf("resolved out-of-range: got %+v, want none", out)
	}
}
