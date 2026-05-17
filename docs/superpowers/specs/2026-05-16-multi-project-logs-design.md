# 多项目日志查询扩展 — 设计文档

**日期:** 2026-05-16
**状态:** 已批准（待用户复核 spec）
**作者:** brainstorming 流程产出

## 1. 背景与目标

`qiniu-logs` 当前只支持一种日志路径布局：用单一 `path_prefix` + `user_id`
组成前缀（`{path_prefix}/{user_id}`），列举该前缀下所有对象，再在客户端按
**对象上传时间 `PutTime`** 过滤时间窗口。

新需求：同一七牛账号/bucket 下存在多个产品，各自有独立的下载路径布局。
典型例子是「直播小助手」(`live_service`)：

```
live_service/12345/20260516_1030/log_12345_20260516_103015_a1b2c3d4.zip
```

布局规律：`live_service/{uid}/{YYYYMMDD_HHMM}/log_{uid}_{YYYYMMDD_HHMMSS}_{hash}.zip`
—— 时间戳**写在路径里**，而不是只能靠对象 `PutTime`。

目标：让工具支持多个项目，每个项目有独立路径布局，仍可按 `uid + 时间范围`
查询，且**对现有用户零行为变更**。

### 已确认的需求边界（来自 brainstorming 澄清）

1. **凭证/Bucket 范围**：所有项目共用同一套 AK/SK + 同一个 bucket。
   project 仅代表「路径布局不同」，不携带独立凭证。
2. **时间口径**：每个项目可配置时间来源。`live_service` 用**路径里的时间戳**；
   现有 `rela-debug-log`（路径无时间）继续用 `PutTime`。
3. **项目选择**：`--project` / `-p` 标志；不传时用 config 里的默认项目；
   默认项目 = 现有 flat 布局，保证向后兼容。
4. **可扩展方式**：纯配置声明模板。新增项目 = 改 `config.yaml`，无需重编译。

### 非目标 (YAGNI)

- 跨项目聚合查询（一次扫所有项目并合并）。
- 每项目独立凭证/bucket/domain。
- 把 `init` 做成多项目交互向导。
- 按日期目录做前缀收敛的列举优化（见 §9 延后项）。

## 2. 关键设计决策

### 2.1 时间提取规则的表达方式：正则 + Go 时间布局（Approach A）

项目通过两个字段声明如何从对象 key 抽取时间：

- `time_regex`：含**恰好一个捕获组**的正则，捕获 key 中的时间戳子串。
- `time_layout`：与捕获子串匹配的 Go 时间布局。

`live_service` 示例：

```yaml
time_regex:  "_(\\d{8}_\\d{6})_"     # 从 log_12345_20260516_103015_a1b2c3d4.zip 捕获 20260516_103015
time_layout: "20060102_150405"
```

**取舍**：相较 strftime 风格单字段模板（需自写 glob+strftime 解析、`{uid}`
重复与 `*` hash 带来歧义），正则+布局只用 Go 标准库 `regexp` 与
`time.ParseInLocation`，新增代码最少、可在配置加载时校验，且加项目仍是纯配置编辑。
命名内置提取器的方案被「纯配置」需求排除。

### 2.2 旧默认项目保持 bug 兼容前缀

合成出的旧默认项目，prefix 模板用 `{uid}`（必要时 `{path_prefix}/{uid}`），
**不带尾部 `/`，逐字保留今天的行为**（含 `123` 也会匹配 `1234` 这一既有特性）。
理由：向后兼容的最强保证是「现有用户行为完全不变」。新项目用显式带 `/`
的模板（如 `live_service/{uid}/`），不受此历史行为影响。

## 3. 配置 Schema（向后兼容）

```yaml
qiniu:
  access_key: ""
  secret_key: ""
  bucket: rela-debug-log
  domain: ""
  use_https: true
  private: true
  path_prefix: ""                  # 旧字段保留，仅用于向后兼容合成默认项目
  default_project: rela-debug-log
  projects:
    rela-debug-log:
      prefix: "{uid}"              # 逐字等价于今天的行为
      time_source: put_time
    live_service:
      prefix: "live_service/{uid}/"
      time_source: path
      time_regex: "_(\\d{8}_\\d{6})_"
      time_layout: "20060102_150405"
```

字段说明：

| 字段 | 必填 | 说明 |
|---|---|---|
| `default_project` | ❌ | 不传 `--project` 时用的项目名。缺省时取合成的旧默认项目 |
| `projects` | ❌ | 项目名 → 项目定义的 map。为空时由加载逻辑合成 |
| `projects.<name>.prefix` | ✅ | 列举前缀模板，必须含 `{uid}` 占位符 |
| `projects.<name>.time_source` | ❌ | `put_time`（默认）\| `path` |
| `projects.<name>.time_regex` | 当 `time_source: path` | 含一个捕获组的正则 |
| `projects.<name>.time_layout` | 当 `time_source: path` | Go 时间布局 |

### 3.1 向后兼容合成规则

`config.Load` 反序列化后，若 `projects` 为空：

1. 合成一个名为 `default` 的项目：
   - `prefix` = 若旧 `path_prefix` 非空则 `{path_prefix}/{uid}`，否则 `{uid}`
     （把旧 `path_prefix` 值原样拼入，保持 §2.2 的逐字行为）
   - `time_source` = `put_time`
2. 若 `default_project` 为空，设为 `default`。

现有用户：配置不动、行为不变、无需感知 `projects` 概念。

## 4. 新增 `internal/project` 包

职责单一：把「项目定义」翻译成「列举前缀」与「单文件时间」。

```go
package project

type TimeSource string
const (
    TimePutTime TimeSource = "put_time"
    TimePath    TimeSource = "path"
)

type Project struct {
    Name       string
    Prefix     string         // 含 {uid} 的模板
    TimeSource TimeSource
    TimeLayout string
    timeRe     *regexp.Regexp // 编译后的 time_regex
}

// ListPrefix 用 uid 替换 {uid} 占位符，得到列举前缀。
func (p *Project) ListPrefix(uid string) string

// FileTime 返回该 key 对应的逻辑时间。
//   put_time: 直接返回传入的 putTime
//   path:     对 key 跑 timeRe，取捕获组用 TimeLayout 在 time.Local 解析
// 无法解析时返回 (zero, error)。
func (p *Project) FileTime(key string, putTime time.Time) (time.Time, error)

// Validate 在配置加载时调用：
//   - Prefix 必须含 {uid}
//   - time_source 必须是已知枚举
//   - time_source==path 时：time_regex 能编译且恰好 1 个捕获组；time_layout 非空
func (p *Project) Validate() error
```

- 输入：项目配置字段。
- 依赖：仅标准库 `regexp` / `time`。
- 不依赖 `config` 或 `qiniu` 包，可独立单测。

## 5. `qiniu.Client.ListFiles` 重构

让 `qiniu` 包与多项目逻辑解耦：

```go
// 旧:
func (c *Client) ListFiles(ctx, userID string, opts ListOptions) ([]FileInfo, error)

// 新:
type TimeResolver func(key string, putTime time.Time) (time.Time, error)
func (c *Client) ListFiles(ctx, prefix string, resolve TimeResolver, opts ListOptions) ([]FileInfo, error)
```

- 调用方（`cmd`）先用 `project.ListPrefix(uid)` 求前缀、
  用 `project.FileTime` 作为 `resolve` 传入。
- `ListFiles` 内部把原来的 `putTime := time.Unix(...)` 替换为
  `fileTime, err := resolve(entry.Key, putTime)`，按 §6 规则处理 err 后再比较时间窗。
- `qiniu` 包不再读取 `cfg.PathPrefix`。
- `FileInfo` 新增 `LogTime time.Time` 字段（解析成功则为逻辑时间，
  失败则等于 `PutTime`），供展示用；`PutTime` 仍保留。

## 6. 时间解析边界规则

对每个列举到的对象，调用 `resolve(key, putTime)`：

| 情况 | 有时间过滤 (`--from/--to/--last`) | 无时间过滤 |
|---|---|---|
| 解析成功 | 按解析时间判断是否在窗口内 | 收录，展示用解析时间 |
| 解析失败（不匹配正则） | **排除**（无法证明在窗口内） | **收录**，展示时间回退到 `PutTime` |

这样：

- `qiniu-logs list <uid> --project live_service`（无时间标志）仍列出全部。
- 带时间标志的查询保持正确，不会把无法定位时间的文件误纳入。

## 7. CLI / UX 变更

- `list`、`search` 新增 `--project` / `-p string`。
  解析顺序：flag → config `default_project` → 合成的旧默认项目。
- 传入未知项目名 → 报错并列出可用项目名。
- `download` 不变（按显式 key 操作，与项目无关）。
- `config` 命令：在现有输出后追加项目表与默认项目，例如：

  ```
  默认项目: rela-debug-log
  项目:
    rela-debug-log   prefix={uid}                time=put_time
    live_service     prefix=live_service/{uid}/  time=path
  ```

- `init`：继续只交互收集凭证 + 旧式 `path_prefix`，写出含**单个默认项目**
  的新 schema。新增更多项目走「改 YAML / AI 写文件」，不扩成交互向导（YAGNI）。

## 8. 文档与测试

### 文档

- `skill/SKILL.md`：
  - 「可被 AI 驱动的命令」表加入 `--project`。
  - 配置初始化章节（§2.x）的 YAML 模板换成新 `projects` schema，
    给出 `live_service` 示例。
  - 故障处理表补充「项目名错误 / 路径布局不匹配」一行。
- `README.md`：配置说明、命令参考、「文件路径规则」章节更新为多项目模型。

### 测试

- `internal/project`：
  - `ListPrefix`：`{uid}` 替换；带/不带尾 `/`。
  - `FileTime` (path)：用 §1 的真实 live_service key 解析出
    `2026-05-16 10:30:15`；正则不匹配返回 error。
  - `FileTime` (put_time)：原样返回 putTime。
  - `Validate`：缺 `{uid}`、正则编译失败、捕获组数 ≠ 1、layout 为空各一条。
- `internal/config`：旧配置（无 `projects`、有/无 `path_prefix`）→ 合成
  `default` 项目且行为等价的回归测试。
- `qiniu.ListFiles`：用假 resolver 验证 §6 四种分支（成功/失败 × 有/无时间过滤）。

## 9. 延后项（非 v1）

- **按日期目录前缀收敛**：对 `time_source: path` 且路径含日期目录的项目，
  可由时间窗口推导候选日期前缀（如 `live_service/{uid}/20260516`），
  减少超大历史用户的列举往返。仅性能优化，不影响正确性，v1 不做。

## 10. 影响面小结

| 文件 | 变更 |
|---|---|
| `internal/config/config.go` | 新增 `Projects`/`DefaultProject` 字段、合成逻辑、校验 |
| `internal/project/project.go` (新) | `Project` 类型与前缀/时间/校验逻辑 |
| `internal/qiniu/client.go` | `ListFiles` 签名改为 prefix+resolver；`FileInfo.LogTime` |
| `cmd/list.go` `cmd/search.go` | `--project` 标志与项目解析 |
| `cmd/config.go` | 打印项目表 |
| `cmd/init.go` | 写新 schema（单默认项目） |
| `internal/ui/model.go` | 适配 `ListFiles` 新签名 / 展示 `LogTime` |
| `skill/SKILL.md` `README.md` | 文档更新 |
| 各 `*_test.go` | 见 §8 |
