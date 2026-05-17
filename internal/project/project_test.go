package project

import (
	"testing"
	"time"
)

func TestListPrefix(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		uid    string
		want   string
	}{
		{"legacy flat", "{uid}", "12345", "12345"},
		{"legacy with prefix", "logs/{uid}", "12345", "logs/12345"},
		{"live_service", "live_service/{uid}/", "12345", "live_service/12345/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Project{Name: c.name, Prefix: c.prefix, TimeSource: TimePutTime}
			if got := p.ListPrefix(c.uid); got != c.want {
				t.Fatalf("ListPrefix(%q) = %q, want %q", c.uid, got, c.want)
			}
		})
	}
}

func TestFileTimePutTime(t *testing.T) {
	put := time.Date(2026, 5, 16, 9, 0, 0, 0, time.Local)
	p := &Project{Name: "x", Prefix: "{uid}", TimeSource: TimePutTime}
	got, err := p.FileTime("12345/app.log", put)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.Equal(put) {
		t.Fatalf("FileTime = %v, want %v", got, put)
	}
}

func TestFileTimePath(t *testing.T) {
	p := &Project{
		Name:       "live_service",
		Prefix:     "live_service/{uid}/",
		TimeSource: TimePath,
		TimeRegex:  `_(\d{8}_\d{6})_`,
		TimeLayout: "20060102_150405",
	}
	if err := p.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	key := "live_service/12345/20260516_1030/log_12345_20260516_103015_a1b2c3d4.zip"
	got, err := p.FileTime(key, time.Time{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := time.Date(2026, 5, 16, 10, 30, 15, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("FileTime = %v, want %v", got, want)
	}

	if _, err := p.FileTime("live_service/12345/garbage.txt", time.Time{}); err == nil {
		t.Fatal("expected error for non-matching key, got nil")
	}

	// Lazy path: FileTime without an explicit Compile() must still work.
	lazy := &Project{
		Name:       "lazy",
		Prefix:     "live_service/{uid}/",
		TimeSource: TimePath,
		TimeRegex:  `_(\d{8}_\d{6})_`,
		TimeLayout: "20060102_150405",
	}
	if _, err := lazy.FileTime(key, time.Time{}); err != nil {
		t.Fatalf("lazy compile path: %v", err)
	}
}

func TestValidate(t *testing.T) {
	bad := []struct {
		name string
		p    Project
	}{
		{"no uid placeholder", Project{Name: "a", Prefix: "live_service/", TimeSource: TimePutTime}},
		{"unknown time source", Project{Name: "b", Prefix: "{uid}", TimeSource: "bogus"}},
		{"path no regex", Project{Name: "c", Prefix: "{uid}", TimeSource: TimePath, TimeLayout: "20060102"}},
		{"path bad regex", Project{Name: "d", Prefix: "{uid}", TimeSource: TimePath, TimeRegex: "([", TimeLayout: "x"}},
		{"path zero groups", Project{Name: "e", Prefix: "{uid}", TimeSource: TimePath, TimeRegex: `\d+`, TimeLayout: "x"}},
		{"path two groups", Project{Name: "f", Prefix: "{uid}", TimeSource: TimePath, TimeRegex: `(\d)(\d)`, TimeLayout: "x"}},
		{"path no layout", Project{Name: "g", Prefix: "{uid}", TimeSource: TimePath, TimeRegex: `(\d+)`}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if err := c.p.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want error for %s", c.name)
			}
		})
	}

	good := Project{Name: "ok", Prefix: "live_service/{uid}/", TimeSource: TimePath, TimeRegex: `_(\d{8}_\d{6})_`, TimeLayout: "20060102_150405"}
	if err := good.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}
