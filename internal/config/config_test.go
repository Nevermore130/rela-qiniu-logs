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

func TestConfigProjectFactory(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: live_service
  projects:
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
	proj, err := cfg.Project("live_service")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if proj.Name != "live_service" || proj.Prefix != "live_service/{uid}/" {
		t.Fatalf("unexpected project: %+v", proj)
	}
	if got := proj.ListPrefix("12345"); got != "live_service/12345/" {
		t.Fatalf("ListPrefix = %q, want live_service/12345/", got)
	}
	if _, err := cfg.Project("ghost"); err == nil {
		t.Fatal("expected error for unknown project name")
	}
}

func TestDefaultConfigHasBuiltinProjects(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Qiniu.DefaultProject != "rela-debug-log" {
		t.Fatalf("DefaultProject = %q, want rela-debug-log", cfg.Qiniu.DefaultProject)
	}
	rd, ok := cfg.Qiniu.Projects["rela-debug-log"]
	if !ok || rd.Prefix != "{uid}" || rd.TimeSource != "put_time" {
		t.Fatalf("rela-debug-log wrong: %+v ok=%v", rd, ok)
	}
	ls, ok := cfg.Qiniu.Projects["live_service"]
	if !ok || ls.Prefix != "live_service/{uid}/" || ls.TimeSource != "path" ||
		ls.TimeRegex != `_(\d{8}_\d{6})_` || ls.TimeLayout != "20060102_150405" {
		t.Fatalf("live_service wrong: %+v ok=%v", ls, ok)
	}
	if _, err := cfg.Project("rela-debug-log"); err != nil {
		t.Fatalf("DefaultConfig not valid: %v", err)
	}
	if _, err := cfg.Project("live_service"); err != nil {
		t.Fatalf("live_service not valid: %v", err)
	}
}

func TestLegacySynthesisInjectsLiveService(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  path_prefix: ""
  use_https: true
  private: true
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Qiniu.DefaultProject != "default" {
		t.Fatalf("DefaultProject = %q, want default", cfg.Qiniu.DefaultProject)
	}
	def, ok := cfg.Qiniu.Projects["default"]
	if !ok || def.Prefix != "{uid}" || def.TimeSource != "put_time" {
		t.Fatalf("default synth changed: %+v ok=%v", def, ok)
	}
	ls, ok := cfg.Qiniu.Projects["live_service"]
	if !ok || ls.Prefix != "live_service/{uid}/" || ls.TimeSource != "path" {
		t.Fatalf("live_service not injected: %+v ok=%v", ls, ok)
	}
}

func TestLegacySynthesisPathPrefixRegressionGuard(t *testing.T) {
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
		t.Fatalf("default prefix = %q, want logs/{uid} (regression!)", got)
	}
	if _, ok := cfg.Qiniu.Projects["live_service"]; !ok {
		t.Fatal("live_service should still be injected alongside path_prefix default")
	}
}

func TestExplicitProjectsNotAutoInjected(t *testing.T) {
	p := writeTmp(t, `
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  default_project: only
  projects:
    only:
      prefix: "{uid}"
      time_source: put_time
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Qiniu.Projects["live_service"]; ok {
		t.Fatal("must NOT auto-inject live_service into a config that declares projects")
	}
	if len(cfg.Qiniu.Projects) != 1 {
		t.Fatalf("got %d projects, want 1 (only)", len(cfg.Qiniu.Projects))
	}
}
