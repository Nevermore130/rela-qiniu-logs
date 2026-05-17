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
