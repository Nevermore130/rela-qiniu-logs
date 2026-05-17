# Multi-Project Log Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `qiniu-logs` query logs for multiple projects that share one Qiniu account+bucket but have independent path layouts, selectable via `--project`, with a backward-compatible default.

**Architecture:** A new `internal/project` package turns a config-declared project (prefix template + time-extraction rule) into a list prefix and a per-file time resolver. `qiniu.ListFiles` is decoupled to take a resolved prefix + a `TimeResolver`. Config gains a `projects` map with backward-compat synthesis from the legacy `path_prefix`. `cmd` resolves the active project from `--project` → config default → synthesized legacy default.

**Tech Stack:** Go 1.21, cobra, qiniu go-sdk v7, gopkg.in/yaml.v3, stdlib `testing` (table-driven), `make test` = `go test -v ./...`.

**Design decisions locked in (from spec):**
- Shared AK/SK + bucket; project = path layout only.
- Time-extraction = regex (exactly one capture group) + Go time layout (Approach A).
- Legacy default project keeps **bug-compatible** prefix (`{uid}`, no trailing slash).
- Unresolvable time + active time filter → exclude; no filter → include, display falls back to `PutTime`.

Spec: `docs/superpowers/specs/2026-05-16-multi-project-logs-design.md`

---

### Task 1: `internal/project` package

**Files:**
- Create: `internal/project/project.go`
- Test: `internal/project/project_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/project/project_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/project/ -v`
Expected: FAIL — `internal/project` package does not exist / undefined `Project`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/project/project.go`:

```go
// Package project translates a configured project definition into a Qiniu
// list prefix and a per-file logical-time resolver. It deliberately depends
// only on the standard library so it can be unit-tested in isolation.
package project

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type TimeSource string

const (
	TimePutTime TimeSource = "put_time"
	TimePath    TimeSource = "path"
)

// Project is one named path layout within the shared account+bucket.
type Project struct {
	Name       string
	Prefix     string // template, must contain "{uid}"
	TimeSource TimeSource
	TimeRegex  string // required when TimeSource == TimePath; exactly one capture group
	TimeLayout string // required when TimeSource == TimePath; Go reference layout

	timeRe *regexp.Regexp
}

const uidPlaceholder = "{uid}"

// ListPrefix substitutes the {uid} placeholder, yielding the Qiniu list prefix.
func (p *Project) ListPrefix(uid string) string {
	return strings.ReplaceAll(p.Prefix, uidPlaceholder, uid)
}

// Compile prepares the time regex. Call once after Validate (Validate also
// compiles, so calling Compile separately is optional but harmless).
func (p *Project) Compile() error {
	if p.TimeSource != TimePath {
		return nil
	}
	re, err := regexp.Compile(p.TimeRegex)
	if err != nil {
		return fmt.Errorf("项目 %q 的 time_regex 无法编译: %w", p.Name, err)
	}
	p.timeRe = re
	return nil
}

// FileTime returns the logical time for a key. For put_time it echoes the
// supplied object PutTime. For path it extracts the regex capture group and
// parses it with TimeLayout in the local timezone.
func (p *Project) FileTime(key string, putTime time.Time) (time.Time, error) {
	if p.TimeSource == TimePutTime {
		return putTime, nil
	}
	if p.timeRe == nil {
		if err := p.Compile(); err != nil {
			return time.Time{}, err
		}
	}
	m := p.timeRe.FindStringSubmatch(key)
	if m == nil || len(m) < 2 {
		return time.Time{}, fmt.Errorf("项目 %q: key %q 不匹配 time_regex", p.Name, key)
	}
	t, err := time.ParseInLocation(p.TimeLayout, m[1], time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("项目 %q: 无法用 time_layout %q 解析 %q: %w", p.Name, p.TimeLayout, m[1], err)
	}
	return t, nil
}

// Validate checks structural correctness and compiles the regex.
func (p *Project) Validate() error {
	if !strings.Contains(p.Prefix, uidPlaceholder) {
		return fmt.Errorf("项目 %q: prefix 必须包含 %s 占位符", p.Name, uidPlaceholder)
	}
	switch p.TimeSource {
	case TimePutTime:
		return nil
	case TimePath:
		if p.TimeRegex == "" {
			return fmt.Errorf("项目 %q: time_source=path 时 time_regex 不能为空", p.Name)
		}
		if p.TimeLayout == "" {
			return fmt.Errorf("项目 %q: time_source=path 时 time_layout 不能为空", p.Name)
		}
		re, err := regexp.Compile(p.TimeRegex)
		if err != nil {
			return fmt.Errorf("项目 %q 的 time_regex 无法编译: %w", p.Name, err)
		}
		if re.NumSubexp() != 1 {
			return fmt.Errorf("项目 %q: time_regex 必须恰好包含 1 个捕获组，当前 %d 个", p.Name, re.NumSubexp())
		}
		p.timeRe = re
		return nil
	default:
		return fmt.Errorf("项目 %q: 未知 time_source %q（应为 put_time 或 path）", p.Name, p.TimeSource)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/project/ -v`
Expected: PASS — all of `TestListPrefix`, `TestFileTimePutTime`, `TestFileTimePath`, `TestValidate`.

- [ ] **Step 5: Commit**

```bash
git add internal/project/project.go internal/project/project_test.go
git commit -m "feat(project): path-layout + time-extraction project model

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Config schema + backward-compat synthesis

**Files:**
- Modify: `internal/config/config.go` (struct `QiniuConfig`, `Load`, `Validate`, `DefaultConfig`)
- Test: `internal/config/config_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTmp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadLegacyConfigSynthesizesDefaultProject(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: rela-debug-log
  domain: cdn.example.com
  path_prefix: ""
  use_https: true
  private: true
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Qiniu.DefaultProject != "default" {
		t.Fatalf("DefaultProject = %q, want \"default\"", cfg.Qiniu.DefaultProject)
	}
	def, ok := cfg.Qiniu.Projects["default"]
	if !ok {
		t.Fatal("expected synthesized \"default\" project")
	}
	if def.Prefix != "{uid}" {
		t.Fatalf("synth prefix = %q, want \"{uid}\"", def.Prefix)
	}
	if def.TimeSource != "put_time" {
		t.Fatalf("synth time_source = %q, want put_time", def.TimeSource)
	}
}

func TestLoadLegacyConfigWithPathPrefix(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  path_prefix: logs
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Qiniu.Projects["default"].Prefix; got != "logs/{uid}" {
		t.Fatalf("synth prefix = %q, want \"logs/{uid}\"", got)
	}
}

func TestLoadMultiProjectConfig(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: rela-debug-log
  projects:
    rela-debug-log:
      prefix: "{uid}"
      time_source: put_time
    live_service:
      prefix: "live_service/{uid}/"
      time_source: path
      time_regex: "_(\\d{8}_\\d{6})_"
      time_layout: "20060102_150405"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Qiniu.Projects) != 2 {
		t.Fatalf("got %d projects, want 2", len(cfg.Qiniu.Projects))
	}
	ls := cfg.Qiniu.Projects["live_service"]
	if ls.Prefix != "live_service/{uid}/" || ls.TimeSource != "path" {
		t.Fatalf("live_service parsed wrong: %+v", ls)
	}
}

func TestLoadRejectsInvalidProject(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: x
  projects:
    x:
      prefix: "no-placeholder/"
      time_source: put_time
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for prefix without {uid}")
	}
}

func TestLoadRejectsUnknownDefaultProject(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: nope
  projects:
    x:
      prefix: "{uid}"
      time_source: put_time
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error: default_project references unknown project")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `DefaultProject`/`Projects` fields undefined on `QiniuConfig`.

- [ ] **Step 3: Write minimal implementation**

In `internal/config/config.go`, replace the `QiniuConfig` struct (lines 15-23) with:

```go
type ProjectConfig struct {
	Prefix     string `yaml:"prefix"`
	TimeSource string `yaml:"time_source"`
	TimeRegex  string `yaml:"time_regex"`
	TimeLayout string `yaml:"time_layout"`
}

type QiniuConfig struct {
	AccessKey      string                   `yaml:"access_key"`
	SecretKey      string                   `yaml:"secret_key"`
	Bucket         string                   `yaml:"bucket"`
	Domain         string                   `yaml:"domain"`
	PathPrefix     string                   `yaml:"path_prefix"` // legacy; only for backward-compat synthesis
	UseHTTPS       bool                     `yaml:"use_https"`
	Private        bool                     `yaml:"private"`
	DefaultProject string                   `yaml:"default_project"`
	Projects       map[string]ProjectConfig `yaml:"projects"`
}
```

Add this import to the existing import block (keep the others):

```go
	"github.com/rela/qiniu-logs/internal/project"
```

In `Load`, insert the synthesis call immediately after `yaml.Unmarshal` succeeds and before `cfg.Validate()` (between current lines 46 and 48):

```go
	cfg.synthesizeDefaultProject()
```

Replace the `Validate` method (current lines 55-69) with:

```go
func (c *Config) synthesizeDefaultProject() {
	if len(c.Qiniu.Projects) > 0 {
		return
	}
	prefix := "{uid}"
	if c.Qiniu.PathPrefix != "" {
		prefix = c.Qiniu.PathPrefix + "/{uid}"
	}
	c.Qiniu.Projects = map[string]ProjectConfig{
		"default": {Prefix: prefix, TimeSource: string(project.TimePutTime)},
	}
	if c.Qiniu.DefaultProject == "" {
		c.Qiniu.DefaultProject = "default"
	}
}

// Project builds a validated *project.Project for the given name.
func (c *Config) Project(name string) (*project.Project, error) {
	pc, ok := c.Qiniu.Projects[name]
	if !ok {
		return nil, fmt.Errorf("未知项目 %q；可用项目: %s", name, c.projectNames())
	}
	p := &project.Project{
		Name:       name,
		Prefix:     pc.Prefix,
		TimeSource: project.TimeSource(pc.TimeSource),
		TimeRegex:  pc.TimeRegex,
		TimeLayout: pc.TimeLayout,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (c *Config) projectNames() string {
	names := make([]string, 0, len(c.Qiniu.Projects))
	for n := range c.Qiniu.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (c *Config) Validate() error {
	if c.Qiniu.AccessKey == "" {
		return fmt.Errorf("配置错误: access_key 不能为空")
	}
	if c.Qiniu.SecretKey == "" {
		return fmt.Errorf("配置错误: secret_key 不能为空")
	}
	if c.Qiniu.Bucket == "" {
		return fmt.Errorf("配置错误: bucket 不能为空")
	}
	if c.Qiniu.Domain == "" {
		return fmt.Errorf("配置错误: domain 不能为空")
	}
	if c.Qiniu.DefaultProject != "" {
		if _, ok := c.Qiniu.Projects[c.Qiniu.DefaultProject]; !ok {
			return fmt.Errorf("配置错误: default_project %q 不在 projects 中", c.Qiniu.DefaultProject)
		}
	}
	for name, pc := range c.Qiniu.Projects {
		p := &project.Project{
			Name:       name,
			Prefix:     pc.Prefix,
			TimeSource: project.TimeSource(pc.TimeSource),
			TimeRegex:  pc.TimeRegex,
			TimeLayout: pc.TimeLayout,
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("配置错误: %w", err)
		}
	}
	return nil
}
```

Add `"sort"` and `"strings"` to the import block in `config.go` (alongside existing `fmt`, `os`, `path/filepath`).

Replace `DefaultConfig` (current lines 93-105) with:

```go
func DefaultConfig() *Config {
	return &Config{
		Qiniu: QiniuConfig{
			AccessKey:      "",
			SecretKey:      "",
			Bucket:         "rela-debug-log",
			Domain:         "",
			UseHTTPS:       true,
			Private:        true,
			DefaultProject: "default",
			Projects: map[string]ProjectConfig{
				"default": {Prefix: "{uid}", TimeSource: string(project.TimePutTime)},
			},
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ ./internal/project/ -v`
Expected: PASS — all config and project tests.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): projects map with backward-compat synthesis

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Decouple `qiniu.ListFiles` (prefix + TimeResolver) and add `LogTime`

**Files:**
- Modify: `internal/qiniu/client.go` (`FileInfo`, `ListFiles`; add `TimeResolver`, `selectFiles`, `rawEntry`)
- Test: `internal/qiniu/client_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/qiniu/client_test.go`:

```go
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
	// "bad" falls back to PutTime for LogTime.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/qiniu/ -v`
Expected: FAIL — `rawEntry`, `selectFiles`, `ListOptions` field usage / undefined symbols.

- [ ] **Step 3: Write minimal implementation**

In `internal/qiniu/client.go`, add `LogTime` to `FileInfo` (the struct at lines 23-29):

```go
type FileInfo struct {
	Key      string
	Size     int64
	MimeType string
	PutTime  time.Time
	LogTime  time.Time // resolved logical time; equals PutTime when unresolved
	Hash     string
}
```

Add, just below the `ListOptions` struct (after current line 37):

```go
// TimeResolver maps an object key + its PutTime to the logical log time.
// A non-nil error means the time could not be determined from the key.
type TimeResolver func(key string, putTime time.Time) (time.Time, error)

type rawEntry struct {
	Key      string
	Size     int64
	MimeType string
	PutTime  time.Time
	Hash     string
}

// selectFiles applies the time window + limit to already-listed entries.
// Spec §6: unresolved time is excluded when a time filter is active, and
// included (LogTime falling back to PutTime) when there is no filter.
func selectFiles(entries []rawEntry, resolve TimeResolver, opts ListOptions) ([]FileInfo, error) {
	hasFilter := !opts.From.IsZero() || !opts.To.IsZero()
	var out []FileInfo
	for _, e := range entries {
		logTime, rerr := resolve(e.Key, e.PutTime)
		if rerr != nil {
			if hasFilter {
				continue
			}
			logTime = e.PutTime
		} else {
			if !opts.From.IsZero() && logTime.Before(opts.From) {
				continue
			}
			if !opts.To.IsZero() && logTime.After(opts.To) {
				continue
			}
		}
		out = append(out, FileInfo{
			Key:      e.Key,
			Size:     e.Size,
			MimeType: e.MimeType,
			PutTime:  e.PutTime,
			LogTime:  logTime,
			Hash:     e.Hash,
		})
		if opts.Limit > 0 && len(out) >= opts.Limit {
			return out, nil
		}
	}
	return out, nil
}
```

Replace the entire `ListFiles` method (current lines 52-106) with:

```go
func (c *Client) ListFiles(ctx context.Context, prefix string, resolve TimeResolver, opts ListOptions) ([]FileInfo, error) {
	hasFilter := !opts.From.IsZero() || !opts.To.IsZero()

	var raw []rawEntry
	marker := ""
	batchLimit := 100
	if opts.Limit > 0 && opts.Limit < batchLimit && !hasFilter {
		batchLimit = opts.Limit
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		entries, _, nextMarker, hasNext, err := c.bucketMgr.ListFiles(
			c.cfg.Bucket, prefix, "", marker, batchLimit,
		)
		if err != nil {
			return nil, fmt.Errorf("列举文件失败: %w", err)
		}

		for _, entry := range entries {
			raw = append(raw, rawEntry{
				Key:      entry.Key,
				Size:     entry.Fsize,
				MimeType: entry.MimeType,
				PutTime:  time.Unix(0, entry.PutTime*100),
				Hash:     entry.Hash,
			})
		}

		// When no time filter is active, stop paging once we have enough
		// raw entries to satisfy the limit (preserves the old early-exit).
		if opts.Limit > 0 && !hasFilter && len(raw) >= opts.Limit {
			break
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}

	return selectFiles(raw, resolve, opts)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/qiniu/ -v`
Expected: PASS — `TestSelectFilesNoFilterIncludesAll`, `TestSelectFilesFilterExcludesUnresolvedAndOutOfRange`, `TestSelectFilesRespectsLimit`.

- [ ] **Step 5: Commit**

```bash
git add internal/qiniu/client.go internal/qiniu/client_test.go
git commit -m "refactor(qiniu): ListFiles takes prefix + TimeResolver; add LogTime

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Shared project-resolution helper in `cmd`

**Files:**
- Create: `cmd/project.go`
- Test: `cmd/project_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/project_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/rela/qiniu-logs/internal/config"
)

func cfgWith(def string, names ...string) *config.Config {
	pc := map[string]config.ProjectConfig{}
	for _, n := range names {
		pc[n] = config.ProjectConfig{Prefix: "{uid}", TimeSource: "put_time"}
	}
	return &config.Config{Qiniu: config.QiniuConfig{DefaultProject: def, Projects: pc}}
}

func TestResolveProjectFlagWins(t *testing.T) {
	c := cfgWith("a", "a", "b")
	p, err := resolveProject(c, "b")
	if err != nil || p.Name != "b" {
		t.Fatalf("got (%v, %v), want project b", p, err)
	}
}

func TestResolveProjectFallsBackToDefault(t *testing.T) {
	c := cfgWith("a", "a", "b")
	p, err := resolveProject(c, "")
	if err != nil || p.Name != "a" {
		t.Fatalf("got (%v, %v), want default project a", p, err)
	}
}

func TestResolveProjectUnknownErrors(t *testing.T) {
	c := cfgWith("a", "a")
	if _, err := resolveProject(c, "ghost"); err == nil {
		t.Fatal("expected error for unknown project")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ -run TestResolveProject -v`
Expected: FAIL — `resolveProject` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/project.go`:

```go
package cmd

import (
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/project"
)

// resolveProject picks the active project: explicit flag wins, else the
// config default. Returns a validated *project.Project.
func resolveProject(cfg *config.Config, flag string) (*project.Project, error) {
	name := flag
	if name == "" {
		name = cfg.Qiniu.DefaultProject
	}
	return cfg.Project(name)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ -run TestResolveProject -v`
Expected: PASS — all three `TestResolveProject*`.

- [ ] **Step 5: Commit**

```bash
git add cmd/project.go cmd/project_test.go
git commit -m "feat(cmd): resolveProject helper (flag > config default)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Wire `list` to projects + new ListFiles + display LogTime

**Files:**
- Modify: `cmd/list.go`

- [ ] **Step 1: Add the `--project` flag and rewire `runList`**

Replace the `var (...)` block (lines 13-18) with:

```go
var (
	listLimit   int
	listFrom    string
	listTo      string
	listLast    string
	listProject string
)
```

In the `init()` of `list.go`, add after the existing `listCmd.Flags()` lines:

```go
	listCmd.Flags().StringVarP(&listProject, "project", "p", "", "项目名（不传则用配置中的 default_project）")
```

Replace the body of `runList` from `cfg, err := config.Load(...)` through the file-listing loop (current lines 52-85) with:

```go
	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	proj, err := resolveProject(cfg, listProject)
	if err != nil {
		return err
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	files, err := client.ListFiles(context.Background(), proj.ListPrefix(userID), proj.FileTime, qiniu.ListOptions{
		Limit: listLimit,
		From:  from,
		To:    to,
	})
	if err != nil {
		return fmt.Errorf("列举文件失败: %w", err)
	}

	if !from.IsZero() || !to.IsZero() {
		fmt.Printf("时间范围: %s ~ %s\n", formatBound(from, "(不限)"), formatBound(to, "(不限)"))
	}

	if len(files) == 0 {
		fmt.Printf("未找到用户 %s 的日志文件（项目: %s）\n", userID, proj.Name)
		return nil
	}

	fmt.Printf("找到 %d 个日志文件（项目: %s）:\n\n", len(files), proj.Name)
	for i, f := range files {
		fmt.Printf("%3d. %s\n", i+1, f.Key)
		fmt.Printf("     大小: %s | 时间: %s\n", qiniu.FormatSize(f.Size), f.LogTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("     URL:  %s\n", client.GetPublicURL(f.Key))
	}
	if cfg.Qiniu.Private {
		fmt.Println("\n(私有空间：URL 直接打开会 401，请用 'qiniu-logs download <key>' 下载——会自动签名)")
	}

	return nil
```

> Note: `proj.FileTime` matches `qiniu.TimeResolver` (`func(string, time.Time) (time.Time, error)`). The displayed `时间` now uses `f.LogTime` (logical time for `path` projects, PutTime for `put_time` / unresolved).

- [ ] **Step 2: Build and run offline checks**

Run: `go build ./... && go vet ./...`
Expected: no errors.

Run: `go run . list --help`
Expected: help text includes `-p, --project string` line.

- [ ] **Step 3: Run the full test suite**

Run: `make test`
Expected: PASS for all packages (no regressions).

- [ ] **Step 4: Commit**

```bash
git add cmd/list.go
git commit -m "feat(list): --project flag; query via project prefix; show LogTime

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Wire `search` + TUI model to the new signature

**Files:**
- Modify: `cmd/search.go`
- Modify: `internal/ui/model.go` (`Model` fields, `NewModel`, `loadFiles`)

- [ ] **Step 1: Update the TUI model to take prefix + resolver**

In `internal/ui/model.go`, in the `Model` struct (around lines 77-79) replace the `userID` / `listOpts` region by adding two fields (keep `userID` for display):

```go
	userID   string
	prefix   string
	resolve  qiniu.TimeResolver
	listOpts qiniu.ListOptions
```

Change `NewModel` (line 111) signature and the struct literal that sets `userID` (line 138). New signature:

```go
func NewModel(client *qiniu.Client, userID, prefix string, resolve qiniu.TimeResolver, destDir string, opts qiniu.ListOptions) Model {
```

In the struct literal inside `NewModel`, alongside `userID:   userID,` add:

```go
		prefix:   prefix,
		resolve:  resolve,
```

Change `loadFiles` (line 163) from:

```go
	files, err := m.client.ListFiles(context.Background(), m.userID, m.listOpts)
```

to:

```go
	files, err := m.client.ListFiles(context.Background(), m.prefix, m.resolve, m.listOpts)
```

(Display strings at lines 127/210/297 keep using `m.userID` unchanged.)

- [ ] **Step 2: Update `search.go` to resolve the project**

In `cmd/search.go`, add to the `var (...)` block (lines 15-20):

```go
	searchProject string
```

In `init()` add after the existing flag registrations:

```go
	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "项目名（不传则用配置中的 default_project）")
```

In `runSearch`, after `cfg, err := config.Load(...)` error check (current line 56) and before `client := qiniu.NewClient(...)`, insert:

```go
	proj, err := resolveProject(cfg, searchProject)
	if err != nil {
		return err
	}
```

Replace the `model := ui.NewModel(...)` line (current line 67) with:

```go
	model := ui.NewModel(client, userID, proj.ListPrefix(userID), proj.FileTime, absOutput, qiniu.ListOptions{From: from, To: to})
```

- [ ] **Step 3: Build and check**

Run: `go build ./... && go vet ./...`
Expected: no errors.

Run: `go run . search --help`
Expected: help text includes `-p, --project string`.

- [ ] **Step 4: Run the full test suite**

Run: `make test`
Expected: PASS (no regressions).

- [ ] **Step 5: Commit**

```bash
git add cmd/search.go internal/ui/model.go
git commit -m "feat(search): --project flag; TUI model lists via prefix+resolver

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: `config` command prints the projects table

**Files:**
- Modify: `cmd/config.go` (`runConfig`)

- [ ] **Step 1: Append project info to `runConfig`**

In `cmd/config.go`, replace the line `fmt.Printf("Private:    %t\n", cfg.Qiniu.Private)` (line 44) with:

```go
	fmt.Printf("Private:    %t\n", cfg.Qiniu.Private)
	fmt.Println()
	fmt.Printf("默认项目: %s\n", cfg.Qiniu.DefaultProject)
	fmt.Println("项目:")
	names := make([]string, 0, len(cfg.Qiniu.Projects))
	for n := range cfg.Qiniu.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := cfg.Qiniu.Projects[n]
		ts := p.TimeSource
		if ts == "" {
			ts = "put_time"
		}
		fmt.Printf("  %-16s prefix=%s  time=%s\n", n, p.Prefix, ts)
	}
```

Add `"sort"` to the import block in `cmd/config.go` (alongside existing `fmt`, `os`).

- [ ] **Step 2: Build and verify offline**

Run: `go build ./... && go vet ./...`
Expected: no errors.

- [ ] **Step 3: Run the full test suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/config.go
git commit -m "feat(config-cmd): print projects table and default project

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: `init` writes the new schema with a single default project

**Files:**
- Modify: `cmd/init.go` (`runInit`)

- [ ] **Step 1: Make `init` populate one default project**

`config.DefaultConfig()` (Task 2) already returns a schema with a `default` project. `init` still prompts for the legacy `path_prefix`; translate that prompt into the default project's prefix before save.

In `cmd/init.go`, replace the `PathPrefix` prompt line (current line 49):

```go
	cfg.Qiniu.PathPrefix = promptWithDefault(reader, "文件路径前缀 (可选，留空表示无前缀)", cfg.Qiniu.PathPrefix, false)
```

with:

```go
	cfg.Qiniu.PathPrefix = promptWithDefault(reader, "文件路径前缀 (可选，留空表示无前缀)", cfg.Qiniu.PathPrefix, false)

	// Translate the legacy path_prefix prompt into the single default project.
	defPrefix := "{uid}"
	if cfg.Qiniu.PathPrefix != "" {
		defPrefix = cfg.Qiniu.PathPrefix + "/{uid}"
	}
	cfg.Qiniu.DefaultProject = "default"
	cfg.Qiniu.Projects = map[string]config.ProjectConfig{
		"default": {Prefix: defPrefix, TimeSource: "put_time"},
	}
```

Replace the closing message line (current line 65):

```go
	fmt.Println("现在可以使用 'qiniu-logs search <user_id>' 搜索日志文件")
```

with:

```go
	fmt.Println("现在可以使用 'qiniu-logs search <user_id>' 搜索日志文件")
	fmt.Println("多项目：编辑 ~/.qiniu-logs/config.yaml 的 projects: 段，再用 --project 选择")
```

- [ ] **Step 2: Build and smoke-test init non-interactively**

Run: `go build ./... && go vet ./...`
Expected: no errors.

Run:
```bash
printf 'ak\nsk\nrela-debug-log\ncdn.example.com\n\ny\n' | go run . --config /tmp/ql-test.yaml init && go run . --config /tmp/ql-test.yaml config && rm -f /tmp/ql-test.yaml
```
Expected: `config` output shows `默认项目: default` and a `default  prefix={uid}  time=put_time` row.

- [ ] **Step 3: Run the full test suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/init.go
git commit -m "feat(init): write projects schema with a single default project

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Documentation — SKILL.md and README.md

**Files:**
- Modify: `skill/SKILL.md`
- Modify: `README.md`

- [ ] **Step 1: Update `skill/SKILL.md`**

a) In the "可被 AI 驱动的命令" table, add a row after the existing `list` rows:

```
| 指定项目（多产品） | `qiniu-logs list <uid> --project live_service --last 24h` |
```

b) Replace the YAML template in section "2.3 YAML 模板" with:

```yaml
qiniu:
  access_key: "<USER_AK>"
  secret_key: "<USER_SK>"
  bucket: "<USER_BUCKET>"
  domain: "<USER_DOMAIN>"
  use_https: true
  private: <true|false>
  default_project: rela-debug-log
  projects:
    rela-debug-log:
      prefix: "{uid}"
      time_source: put_time
    live_service:
      prefix: "live_service/{uid}/"
      time_source: path
      time_regex: "_(\\d{8}_\\d{6})_"
      time_layout: "20060102_150405"
```

c) Add a row to the "故障处理速查" table:

```
| `未知项目 "X"；可用项目: ...` | `--project` 名字写错；用列出的名字之一，或在 config 的 projects 段新增 |
```

d) Replace the "配置文件结构" reference YAML at the end with the same multi-project schema as (b), and add one line below it:

```
新增项目：在 projects: 下加一段，prefix 必须含 {uid}；时间在路径里则用 time_source: path + time_regex(1个捕获组) + time_layout。
```

- [ ] **Step 2: Update `README.md`**

a) In the "## 配置" section, replace the bullet list of fields with:

```
- Access Key / Secret Key / Bucket / Domain: 七牛凭证与存储
- Private: 是否私有空间
- projects: 各产品的路径布局（prefix 模板 + 时间来源）
- default_project: 不传 --project 时使用的项目
```

b) Replace the entire "## 文件路径规则" section body with:

```
工具按「项目」决定搜索路径。每个项目在 config.yaml 的 `projects:` 下声明：

- `prefix`: 含 `{uid}` 占位符的前缀模板，例如 `{uid}`（默认）或 `live_service/{uid}/`
- `time_source`: `put_time`（按对象上传时间，默认）或 `path`（从 key 解析时间）
- `time_regex` / `time_layout`: 当 `time_source: path` 时，用一个捕获组的正则
  抓出时间子串，再用 Go 时间布局解析

查询时用 `--project <name>` 选择；不传则用 `default_project`。
旧配置（只有 `path_prefix`）会自动合成一个 `default` 项目，行为不变。
```

c) In the "## 命令参考 > 全局选项 / 命令" area, add to the command options note:

```
list / search 支持 `-p, --project <name>` 选择项目
```

- [ ] **Step 3: Verify docs build nothing but stay consistent**

Run: `grep -n "default_project" skill/SKILL.md README.md`
Expected: both files reference `default_project`.

- [ ] **Step 4: Commit**

```bash
git add skill/SKILL.md README.md
git commit -m "docs: document multi-project config, --project, live_service example

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Full verification + backward-compat smoke

**Files:** none (verification only)

- [ ] **Step 1: Full build, vet, and test**

Run: `go build ./... && go vet ./... && make test`
Expected: build clean, vet clean, all tests PASS across `internal/project`, `internal/config`, `internal/qiniu`, `cmd`.

- [ ] **Step 2: Backward-compat smoke (legacy config, no projects)**

Run:
```bash
cat > /tmp/ql-legacy.yaml <<'EOF'
qiniu:
  access_key: ak
  secret_key: sk
  bucket: rela-debug-log
  domain: cdn.example.com
  path_prefix: ""
  use_https: true
  private: true
EOF
go run . --config /tmp/ql-legacy.yaml config
rm -f /tmp/ql-legacy.yaml
```
Expected: no load error; output shows `默认项目: default` and `default  prefix={uid}  time=put_time` — proving old configs keep working with zero edits.

- [ ] **Step 3: Multi-project config smoke (offline, validation only)**

Run:
```bash
cat > /tmp/ql-multi.yaml <<'EOF'
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: rela-debug-log
  projects:
    rela-debug-log:
      prefix: "{uid}"
      time_source: put_time
    live_service:
      prefix: "live_service/{uid}/"
      time_source: path
      time_regex: "_(\\d{8}_\\d{6})_"
      time_layout: "20060102_150405"
EOF
go run . --config /tmp/ql-multi.yaml config
rm -f /tmp/ql-multi.yaml
```
Expected: lists both `rela-debug-log` and `live_service` projects; no validation error.

- [ ] **Step 4: Final commit (if any verification fixups were needed)**

```bash
git status --porcelain
# If clean, nothing to do. Otherwise commit the fixups:
git add -A
git commit -m "chore: verification fixups for multi-project support

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| §2.1 regex+layout time extraction | Task 1 (`FileTime` path, `Validate`) |
| §2.2 bug-compatible legacy prefix (`{uid}`, no slash) | Task 2 (`synthesizeDefaultProject`), Task 8 (`init`) |
| §3 config schema | Task 2 (`QiniuConfig`/`ProjectConfig`) |
| §3.1 backward-compat synthesis | Task 2 + Task 10 Step 2 smoke |
| §4 `internal/project` package | Task 1 |
| §5 `ListFiles` prefix+resolver, `FileInfo.LogTime` | Task 3 |
| §6 time-resolution edge rules | Task 3 (`selectFiles` + tests) |
| §7 `--project` on list/search, resolution order | Task 4, 5, 6 |
| §7 `download` unchanged | (no task — intentionally untouched) |
| §7 `config` prints projects | Task 7 |
| §7 `init` single default project | Task 8 |
| §8 docs | Task 9 |
| §8 tests (project/config/qiniu) | Tasks 1, 2, 3 |
| §9 deferred (date-dir narrowing) | intentionally not implemented |

No gaps.

**2. Placeholder scan:** No "TBD/TODO/handle edge cases/similar to Task N". Every code step has full code.

**3. Type consistency:**
- `qiniu.TimeResolver = func(string, time.Time) (time.Time, error)` (Task 3) ⇄ `project.FileTime(key string, putTime time.Time) (time.Time, error)` (Task 1) — signatures match, so `proj.FileTime` is passed directly in Tasks 5/6.
- `config.Project(name) (*project.Project, error)` (Task 2) used by `resolveProject` (Task 4) and consumed in Tasks 5/6.
- `project.Project` fields (`Name/Prefix/TimeSource/TimeRegex/TimeLayout`) consistent across Tasks 1, 2, 4.
- `ui.NewModel(client, userID, prefix, resolve, destDir, opts)` (Task 6) call site matches new definition.
- `config.ProjectConfig` field names (`Prefix/TimeSource/TimeRegex/TimeLayout`, yaml tags) consistent across Tasks 2, 7, 8 and the test in Task 4.

Consistent. Plan ready.
