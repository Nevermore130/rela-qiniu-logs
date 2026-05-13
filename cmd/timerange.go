package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var timeLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

func parseTimeArg(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q（支持: 2006-01-02 / 2006-01-02 15:04 / 2006-01-02 15:04:05 / RFC3339）", s)
}

func parseDurationArg(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	expanded := expandDayUnit(s)
	d, err := time.ParseDuration(expanded)
	if err != nil {
		return 0, fmt.Errorf("无法解析时长 %q（示例: 30m / 24h / 7d / 1h30m）: %w", s, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("时长必须为正: %q", s)
	}
	return d, nil
}

// expandDayUnit 把 `\d+d` 子串展开成等价的 `\d+h`，让 time.ParseDuration 能识别。
// 例如 "7d" -> "168h"，"1d12h" -> "24h12h"（ParseDuration 接受多个相同单位求和）。
func expandDayUnit(s string) string {
	var sb strings.Builder
	n := len(s)
	for i := 0; i < n; {
		j := i
		for j < n && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > i && j < n && s[j] == 'd' {
			num, _ := strconv.Atoi(s[i:j])
			sb.WriteString(strconv.Itoa(num * 24))
			sb.WriteByte('h')
			i = j + 1
			continue
		}
		if j > i {
			sb.WriteString(s[i:j])
			i = j
		} else {
			sb.WriteByte(s[i])
			i++
		}
	}
	return sb.String()
}

// resolveTimeRange 合并 --from / --to / --last 三个 flag。
// --last 与 --from 互斥；未指定的边界保持 zero value，由调用方视作不限。
func resolveTimeRange(fromStr, toStr, lastStr string, now time.Time) (from, to time.Time, err error) {
	if lastStr != "" {
		if fromStr != "" {
			return time.Time{}, time.Time{}, fmt.Errorf("--last 与 --from 不能同时使用")
		}
		d, err := parseDurationArg(lastStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = now.Add(-d)
	} else if fromStr != "" {
		from, err = parseTimeArg(fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if toStr != "" {
		to, err = parseTimeArg(toStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("--to (%s) 不能早于 --from (%s)",
			to.Format(time.RFC3339), from.Format(time.RFC3339))
	}
	return from, to, nil
}

func formatBound(t time.Time, fallback string) string {
	if t.IsZero() {
		return fallback
	}
	return t.Format("2006-01-02 15:04:05")
}
