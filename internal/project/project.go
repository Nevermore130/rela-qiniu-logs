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
