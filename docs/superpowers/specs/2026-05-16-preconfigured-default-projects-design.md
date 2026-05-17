# 预置默认项目 rela-debug-log + live_service — 设计文档

**日期:** 2026-05-16
**状态:** 已批准
**前序:** [2026-05-16-multi-project-logs-design.md](./2026-05-16-multi-project-logs-design.md)

## 1. 背景与目标

多项目能力已上线，但全新安装的用户配置里只有一个 `default` 项目，
`live_service` 需要用户手动编辑 `config.yaml` 才能用。目标：让用户**安装即用**，
`projects` 默认就配好 `rela-debug-log` 与 `live_service`，无需手填。

### 已确认的需求边界（brainstorming 澄清）

1. `init` **去掉 path_prefix 交互提问**；始终预置两个固定项目，
   `default_project=rela-debug-log`。
2. 现有用户的旧配置（只有 `path_prefix`、无 `projects`）在 `Load` 时
   **保留原 `default` 合成不变**（已证明 byte 级 SAFE），并**额外注入** `live_service`。

### 非目标 (YAGNI)

- 不把旧配置里合成的 `default` 改名为 `rela-debug-log`（会动到老用户的
  path_prefix 行为 / 破坏 `--project default` 与脚本）。
- 不向「已显式声明 `projects:`」的配置自动注入任何项目。
- 不改 `internal/project`、`internal/qiniu`、`cmd/list.go`、`cmd/search.go`。

## 2. 关键设计决策

### 2.1 单一事实来源 `config.builtinProjects()`

新增一个返回两个标准项目定义的函数，`DefaultConfig` / 旧配置注入 / 文档模板
都引用它，避免三处定义漂移：

| 项目 | prefix | time_source | time_regex | time_layout |
|---|---|---|---|---|
| `rela-debug-log` | `{uid}` | `put_time` | — | — |
| `live_service` | `live_service/{uid}/` | `path` | `_(\d{8}_\d{6})_` | `20060102_150405` |

`default_project` 默认 `rela-debug-log`。

### 2.2 全新安装路径

- `DefaultConfig()` 返回两个 builtin 项目 + `default_project: rela-debug-log`。
- `cmd/init.go`：删除 path_prefix 提问行与 Task 8 的「translation block」
  （`defPrefix`/`DefaultProject="default"`/单 default map）。新配置直接继承
  `DefaultConfig()` 的两个项目；`init` 仅交互 AK/SK/Bucket/Domain/Private。
  既有配置分支（`existingCfg != nil` 时 `cfg = existingCfg`）不动 ——
  重跑 `init` 会保留该用户已有的 `projects`。结尾提示语更新。

### 2.3 旧配置迁移路径（向后兼容核心）

`synthesizeDefaultProject()`：

- 守卫 `if len(Projects) > 0 { return }` **保留** —— 已显式声明 `projects:`
  的配置原样尊重，不注入。
- 旧分支（无 `projects:`）：
  1. **保持** `default` 合成**完全不变**（`{uid}` 或 `{path_prefix}/{uid}`，
     `put_time`；`default_project` 为空时设为 `default`）—— 这是已证明 SAFE
     的回归基线。
  2. **额外**注入 `live_service`（取自 `builtinProjects()`），仅当尚不存在。

净效果：老用户 `qiniu-logs list 12345` 行为零变化（仍走 `default`）；
`--project live_service` 成为额外能力，无需改配置。纯增量 → 仍 SAFE。

### 2.4 接受的非对称

旧配置迁移后为 `{default, live_service}`（default_project=`default`）；
全新配置为 `{rela-debug-log, live_service}`（default_project=`rela-debug-log`）。
`path_prefix` 为空时 `default` 与 `rela-debug-log` 功能等价。刻意不统一，
以零风险保住老用户行为。

## 3. 影响面

| 文件 | 变更 |
|---|---|
| `internal/config/config.go` | `builtinProjects()`；`DefaultConfig` 用两项目；`synthesizeDefaultProject` 旧分支额外注入 live_service |
| `internal/config/config_test.go` | DefaultConfig 双项目；旧合成含 default+live_service 且 default 前缀不变；显式 projects 不被注入 |
| `cmd/init.go` | 删 path_prefix 提问 + translation block；更新结尾提示 |
| `README.md` / `skill/SKILL.md` | init 不再问 path_prefix、默认带两项目；字段表与提问数（6→5）更新 |

## 4. 测试要点

- `DefaultConfig()` 含 `rela-debug-log` + `live_service`，`default_project=rela-debug-log`，且通过 `Validate()`。
- 旧配置（空 path_prefix）合成 → `Projects` 含 `default`（prefix `{uid}`）+ `live_service`；`default_project=default`。
- 旧配置（`path_prefix: logs`）→ `default` prefix 仍 `logs/{uid}`（回归守卫）+ `live_service` 注入。
- 已声明 `projects:`（无 live_service）的配置 Load 后**不**被注入 live_service（尊重守卫）。
- `init` 非交互冒烟：仅 5 个提问字段（AK/SK/Bucket/Domain/Private），保存的 YAML 含两个 builtin 项目，`config` 视图列出两项目。
- 全量 `go build ./... && go vet ./... && make test` 绿；既有多项目/向后兼容冒烟仍通过。
