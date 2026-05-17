# Preconfigured Default Projects Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Ship `rela-debug-log` + `live_service` projects preconfigured so users get them on fresh install (and old configs additively gain `live_service`), without manual `config.yaml` edits.

**Architecture:** One `config.builtinProjects()` source of truth, consumed by `DefaultConfig()` and by the legacy-synthesis path (additive `live_service` injection while keeping the proven-SAFE `default` synthesis byte-identical). `init` drops the `path_prefix` prompt.

**Tech Stack:** Go 1.21, stdlib `testing`, `make test` = `go test -v ./...`. Commit to `main`. Commit trailer: `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.

Spec: `docs/superpowers/specs/2026-05-16-preconfigured-default-projects-design.md`

---

### Task 1: `builtinProjects()` + `DefaultConfig` + additive legacy injection

**Files:**
- Modify: `internal/config/config.go` (`DefaultConfig`, `synthesizeDefaultProject`; add `builtinProjects`)
- Test: `internal/config/config_test.go` (extend)

- [ ] **Step 1: Write the failing tests** — append to `internal/config/config_test.go`:

```go
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
	// default synthesis unchanged (regression guard)
	if cfg.Qiniu.DefaultProject != "default" {
		t.Fatalf("DefaultProject = %q, want default", cfg.Qiniu.DefaultProject)
	}
	def, ok := cfg.Qiniu.Projects["default"]
	if !ok || def.Prefix != "{uid}" || def.TimeSource != "put_time" {
		t.Fatalf("default synth changed: %+v ok=%v", def, ok)
	}
	// live_service additively injected
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestDefaultConfigHasBuiltinProjects|TestLegacySynthesisInjectsLiveService|TestLegacySynthesisPathPrefixRegressionGuard|TestExplicitProjectsNotAutoInjected' -v`
Expected: FAIL (DefaultConfig still single `default`; no live_service injection).

- [ ] **Step 3: Implement** — in `internal/config/config.go`:

(a) Add the single source of truth (place near `DefaultConfig`):

```go
// builtinProjects returns the projects shipped preconfigured: a plain
// uid-prefixed project plus the live_service path-timestamp layout.
func builtinProjects() map[string]ProjectConfig {
	return map[string]ProjectConfig{
		"rela-debug-log": {
			Prefix:     "{uid}",
			TimeSource: string(project.TimePutTime),
		},
		"live_service": {
			Prefix:     "live_service/{uid}/",
			TimeSource: string(project.TimePath),
			TimeRegex:  `_(\d{8}_\d{6})_`,
			TimeLayout: "20060102_150405",
		},
	}
}
```

(b) Replace the body of `DefaultConfig()` so it uses both builtins:

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
			DefaultProject: "rela-debug-log",
			Projects:       builtinProjects(),
		},
	}
}
```

(c) In `synthesizeDefaultProject()`, KEEP the existing `if len(c.Qiniu.Projects) > 0 { return }` guard and the existing `default` synthesis EXACTLY as-is. Immediately AFTER the existing code that sets `c.Qiniu.Projects = map[string]ProjectConfig{"default": {...}}` and the `if c.Qiniu.DefaultProject == "" { c.Qiniu.DefaultProject = "default" }` line, append the additive injection (still inside the same function, after the default is established):

```go
	if _, ok := c.Qiniu.Projects["live_service"]; !ok {
		c.Qiniu.Projects["live_service"] = builtinProjects()["live_service"]
	}
```

Do NOT change the `default` prefix logic, the guard, or `default_project` handling. Do NOT remove the `PathPrefix` field.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — the 4 new tests plus all pre-existing config tests (`TestLoadLegacyConfigSynthesizesDefaultProject`, `TestLoadLegacyConfigWithPathPrefix`, `TestLoadMultiProjectConfig`, `TestLoadRejectsInvalidProject`, `TestLoadRejectsUnknownDefaultProject`, `TestConfigProjectFactory`). If `TestLoadLegacyConfigSynthesizesDefaultProject` / `TestLoadLegacyConfigWithPathPrefix` now also see a `live_service` key, that is expected and they should still pass (they only assert the `default` project's fields, not project count) — confirm they pass; if either asserts an exact project-count that breaks, report it (do NOT weaken a regression assertion without flagging).

- [ ] **Step 5: Build/vet + commit**

Run: `go build ./... && go vet ./...` (clean).
```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): preconfigure rela-debug-log + live_service; inject live_service into legacy configs

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `init` drops the `path_prefix` prompt

**Files:**
- Modify: `cmd/init.go` (`runInit`)

- [ ] **Step 1: Read `cmd/init.go` fully** to anchor exact current text. The Task-8 era code has, after the Domain prompt: a `path_prefix` prompt line `cfg.Qiniu.PathPrefix = promptWithDefault(reader, "文件路径前缀 ...", cfg.Qiniu.PathPrefix, false)` followed by the translation block (a comment `// Translate the legacy path_prefix prompt ...`, `defPrefix := "{uid}"`, `if cfg.Qiniu.PathPrefix != "" { defPrefix = ... }`, `cfg.Qiniu.DefaultProject = "default"`, `cfg.Qiniu.Projects = map[string]config.ProjectConfig{"default": {...}}`). There is a Private prompt after it, then `cfg.Save`, then closing `fmt.Println` lines (including the `多项目：编辑 ...` hint added in Task 8).

- [ ] **Step 2: Implement** — make these exact edits:

(a) DELETE the entire `path_prefix` prompt line AND the entire translation block (from the `// Translate the legacy path_prefix prompt into the single default project.` comment through the closing `}` of the `cfg.Qiniu.Projects = map[string]config.ProjectConfig{ "default": {...} }` literal). Rationale: for a brand-new config `cfg = config.DefaultConfig()` now already carries `rela-debug-log` + `live_service`; for an existing config the `existingCfg` branch already set `cfg = existingCfg`, so its projects are preserved. The Private prompt that followed the deleted block must remain and now directly follows the Domain prompt.

(b) Replace the closing hint line that currently reads:
```go
	fmt.Println("多项目：编辑 ~/.qiniu-logs/config.yaml 的 projects: 段，再用 --project 选择")
```
with:
```go
	fmt.Println("已预置项目 rela-debug-log / live_service，用 --project 选择；如需更多项目编辑 ~/.qiniu-logs/config.yaml 的 projects: 段")
```

(c) If, after deleting the translation block, the `config` import becomes used only via `config.DefaultConfig()` / `config.Load` (it will still be used — `cfg := config.DefaultConfig()` and `config.Load` remain), keep the import. Run `goimports`/`gofmt` mentally: ensure no unused imports. Do not touch other prompts (AccessKey/SecretKey/Bucket/Domain/Private), the existing-config detection, or `promptWithDefault`.

- [ ] **Step 3: Build + smoke**

Run: `gofmt -l cmd/init.go` (empty), `go build ./...` (clean), `go vet ./...` (clean), `make test` (all PASS).

Non-interactive smoke (NOTE: only 5 prompts now — AccessKey, SecretKey, Bucket, Domain, Private; NO path_prefix):
```bash
printf 'ak\nsk\nrela-debug-log\ncdn.example.com\ny\n' | go run . --config /tmp/pc8.yaml init
echo "---- saved ----"; cat /tmp/pc8.yaml
echo "---- config ----"; go run . --config /tmp/pc8.yaml config
rm -f /tmp/pc8.yaml
```
Expected: saved YAML has `default_project: rela-debug-log` and a `projects:` map containing BOTH `rela-debug-log` (`prefix: '{uid}'`, `time_source: put_time`) and `live_service` (`prefix: live_service/{uid}/`, `time_source: path`, the regex + layout). `config` view shows `默认项目: rela-debug-log` and both project rows. Closing text shows the new preconfigured hint. Paste saved YAML + config view. If the printf field order doesn't match the actual prompts in init.go, adjust to the real order and report it.

- [ ] **Step 4: Commit**

```bash
git add cmd/init.go
git commit -m "feat(init): drop path_prefix prompt; ship preconfigured projects

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Docs — README.md + skill/SKILL.md

**Files:**
- Modify: `README.md`, `skill/SKILL.md`

- [ ] **Step 1: Read both files** (current working-tree state) to anchor exact text. Do NOT revert prior intentional refinements; make only the targeted edits below.

- [ ] **Step 2: `README.md` edits**

(a) In the "## 配置" section, the `按提示输入：` list currently ends at `- Private:` followed by a `> init 只交互收集上面这些字段...` blockquote. Update that blockquote to reflect that `path_prefix` is no longer prompted and both projects ship preconfigured. Replace the existing blockquote text with exactly:
```
> `init` 只交互收集上面这些字段（不再询问 path_prefix），并自动写入预置项目 `rela-debug-log` 与 `live_service`，`default_project` 为 `rela-debug-log`。
> 多项目：完成 `init` 后用 `--project <name>` 选择；需要更多项目时编辑 `~/.qiniu-logs/config.yaml` 的 `projects:` 段。旧配置（只有 `path_prefix`，无 `projects`）在加载时保留原行为并自动追加 `live_service`。
```

- [ ] **Step 3: `skill/SKILL.md` edits**

(a) In the section "### 2.1 向用户收集的字段", the table currently lists access_key/secret_key/bucket/domain/private (5 rows) and prose mentions `path_prefix` is legacy. Update the prose line that mentions `path_prefix` so it states: AI 写配置时直接用 2.3 模板里的 `projects`（已含 `rela-debug-log` + `live_service`）+ `default_project: rela-debug-log`；`path_prefix` 仅为旧配置兼容字段，新配置不写。Keep the AK/SK security guidance intact and unchanged.

(b) In section "### 2.4 写完立刻收紧权限并验证", the sentence about readiness (already fixed in a prior commit to read `看到 access_key / secret_key / bucket / domain 非空且 \`默认项目:\` 有值即就绪`) — verify it is accurate and leave it; only adjust if it still says a stale numeric count.

(c) If there is a `qiniu-logs init` reference anywhere in SKILL.md (e.g. §2.5 退路) implying it prompts for `path_prefix`, ensure no such claim exists; `init` now asks only AK/SK/Bucket/Domain/Private. Do not otherwise restructure §2.5.

- [ ] **Step 4: Verify + commit**

Run: `grep -n "rela-debug-log" README.md skill/SKILL.md` (both reference it), `grep -n "live_service" README.md skill/SKILL.md` (both reference it), `go build ./...` (clean — sanity, no code touched).
```bash
git add README.md skill/SKILL.md
git commit -m "docs: init ships preconfigured projects, no path_prefix prompt

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Build/vet/test**

Run: `go build ./... && go vet ./... && make test`
Expected: clean build/vet; ALL packages PASS (internal/config, internal/project, internal/qiniu, cmd).

- [ ] **Step 2: Fresh-install smoke**

```bash
printf 'AK\nSK\nrela-debug-log\ncdn.example.com\ny\n' | go run . --config /tmp/pcv.yaml init >/dev/null
go run . --config /tmp/pcv.yaml config
rm -f /tmp/pcv.yaml
```
Expected: `默认项目: rela-debug-log`, rows for `rela-debug-log` (prefix={uid} time=put_time) and `live_service` (prefix=live_service/{uid}/ time=path).

- [ ] **Step 3: Legacy-config backward-compat smoke (additive injection, default unchanged)**

```bash
cat > /tmp/pcl.yaml <<'EOF'
qiniu:
  access_key: ak
  secret_key: sk
  bucket: rela-debug-log
  domain: cdn.example.com
  path_prefix: ""
  use_https: true
  private: true
EOF
go run . --config /tmp/pcl.yaml config
rm -f /tmp/pcl.yaml
```
Expected: `默认项目: default`; rows for `default` (prefix={uid} time=put_time) AND `live_service`. Proves old behavior preserved (`default` still default + `{uid}`) while `live_service` is additively available.

- [ ] **Step 4: Legacy with path_prefix regression smoke**

```bash
cat > /tmp/pcp.yaml <<'EOF'
qiniu:
  access_key: ak
  secret_key: sk
  bucket: b
  domain: d
  path_prefix: logs
EOF
go run . --config /tmp/pcp.yaml config
rm -f /tmp/pcp.yaml
```
Expected: `default` row prefix=`logs/{uid}` (UNCHANGED — regression guard) plus `live_service` row.

- [ ] **Step 5: Report**

`git log --oneline -8` and `git status --porcelain` (clean). Report PASS/FAIL with pasted evidence for steps 1–4. Do not modify files.

---

## Self-Review

**Spec coverage:**
| Spec section | Task |
|---|---|
| §2.1 `builtinProjects()` source of truth | Task 1a |
| §2.2 DefaultConfig two projects | Task 1b + Task 1 test |
| §2.2 init drops path_prefix prompt | Task 2 |
| §2.3 legacy keeps `default` SAFE + additively injects live_service | Task 1c + Task 1 tests + Task 4 steps 3-4 |
| §2.3 explicit `projects:` not injected (guard kept) | Task 1 `TestExplicitProjectsNotAutoInjected` |
| §2.4 accepted asymmetry | inherent in Task 1 (no rename); verified by smokes |
| §3/§4 docs + tests + verification | Tasks 3, 4 |
No gaps.

**Placeholder scan:** none. Every code/test step has full content.

**Type consistency:** `builtinProjects() map[string]ProjectConfig` returns the same `ProjectConfig` type consumed by `DefaultConfig` and `synthesizeDefaultProject`; `project.TimePutTime`/`project.TimePath` already imported in config.go (used by prior synthesis). `Config.Project(name)` validation (existing) exercises the builtin live_service regex (1 capture group) — valid. Consistent.
