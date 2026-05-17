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

func raw(key string) rawEntry {
	return rawEntry{Key: key, Size: 1, PutTime: tm("2026-05-16 09:00:00")}
}

func TestSelectFilesNoFilterIncludesAll(t *testing.T) {
	in := []rawEntry{raw("good-early"), raw("bad"), raw("good-late")}
	out, err := selectFiles(in, fakeResolver, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
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
	in := []rawEntry{raw("good-early"), raw("bad"), raw("good-late")}
	opts := ListOptions{From: tm("2026-05-15 00:00:00")}
	out, err := selectFiles(in, fakeResolver, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Key != "good-late" {
		t.Fatalf("got %+v, want only good-late", out)
	}
}

func TestSelectFilesRespectsLimit(t *testing.T) {
	in := []rawEntry{raw("good-early"), raw("good-late"), raw("other")}
	out, err := selectFiles(in, fakeResolver, ListOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d, want 2 (limit)", len(out))
	}
}
