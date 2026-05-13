---
name: qiniu-logs
description: >
  通过 qiniu-logs CLI 在七牛云对象存储中按用户ID + 时间范围
  查询并下载用户的日志文件。

  Use when the user asks to:
  - 查找/下载/拉取某用户在云端的日志文件（"下载用户 12345 最近 24h 的日志"、"查 user 12345 昨天的报错日志"）
  - 排查用户问题需要先从云端日志开始
  - 提及 七牛 / qiniu / rela-debug-log / kodo / qiniu-logs

  Do NOT use this skill when the user wants application logs on the local
  machine, server-side stdout, or anything not stored in 七牛 Kodo.
triggers:
  keywords:
    - 日志
    - log
    - logs
    - 七牛
    - qiniu
    - kodo
    - rela-debug-log
    - qiniu-logs
  patterns:
    - 用户.+(日志|log)
    - (查|看|下载|拉取|获取|找).+(日志|log)
    - 日志.+(下载|拉取)
    - uid\s*\d+.+(日志|log)
metadata:
  cli: qiniu-logs
  install: brew install Nevermore130/rela-qiniu-logs/qiniu-logs
  repo: https://github.com/Nevermore130/rela-qiniu-logs
  config_path: ~/.qiniu-logs/config.yaml
---

# qiniu-logs — 七牛云日志查询与下载

按 `user_id` + 时间范围列举 / 下载日志文件。底层 CLI：`qiniu-logs`。

## 进入流程的前置检查（**必跑**）

每次接到日志查询请求，先按顺序确认三件事，缺一项就先补齐再继续：

1. **CLI 已安装？**
   ```bash
   command -v qiniu-logs >/dev/null && qiniu-logs --version || echo MISSING
   ```

   若 `MISSING`，**AI agent 可以自己执行下面的安装命令**（修改系统状态，按各 agent 的"破坏性动作前先告知一句"约定通告即可；除非碰到 sudo 提示，否则无需用户输入）。按优先级走：

   **A. 有 brew（macOS / Linuxbrew，首选）**
   ```bash
   command -v brew >/dev/null && brew install Nevermore130/rela-qiniu-logs/qiniu-logs
   ```
   `brew install` 不需要 sudo，落到 `$(brew --prefix)/bin`，PATH 通常已包含。装完用 `qiniu-logs --version` 自检。

   **B. 没装 brew 但用户想装**
   下面这条会调 sudo，**AI 不要直接执行**，让用户自己跑：
   ```bash
   /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
   ```
   完成后回到 A。

   **C. 完全跳过 brew（不需要 sudo 的 fallback）**
   AI 可直接执行；装到 `~/.local/bin` 避开权限问题：
   ```bash
   set -e
   TAG=$(curl -fsSL https://api.github.com/repos/Nevermore130/rela-qiniu-logs/releases/latest \
           | grep -oE '"tag_name": *"v[^"]+"' \
           | head -1 \
           | sed 's/.*"v\([^"]*\)".*/\1/')
   OS=$(uname -s | tr '[:upper:]' '[:lower:]')                          # darwin | linux
   ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && ARCH=amd64; [ "$ARCH" = "aarch64" ] && ARCH=arm64
   URL="https://github.com/Nevermore130/rela-qiniu-logs/releases/download/v${TAG}/rela-qiniu-logs_${TAG}_${OS}_${ARCH}.tar.gz"
   mkdir -p "$HOME/.local/bin"
   curl -fsSL "$URL" | tar -xz -C "$HOME/.local/bin" qiniu-logs
   case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) echo "⚠️  请把 \$HOME/.local/bin 加入 PATH"; esac
   "$HOME/.local/bin/qiniu-logs" --version
   ```

   ⛔ **不要** `go install`：会绕过发布管道、拿到未发版 HEAD，且不带 ldflags 注入的 version。

2. **配置已就绪？**
   ```bash
   test -f ~/.qiniu-logs/config.yaml && qiniu-logs config || echo NO_CONFIG
   ```

   若 `NO_CONFIG`，**AI 自己驱动初始化**，流程：用 agent 的"向用户提问"原语（Claude Code: `AskUserQuestion`；Codex / Qoder: 等价的交互输入）**逐项收集**字段 → 用 agent 的"文件写入"原语直接落到 `~/.qiniu-logs/config.yaml` → `qiniu-logs config` 验证。

   ### 2.1 向用户收集的 6 个字段

   | 字段 | 必填 | 提问示例 | 备注 |
   |---|---|---|---|
   | `access_key` | ✅ | "请输入七牛 Access Key" | 敏感凭证，**收到即写文件，不在后续总结/对话里复述** |
   | `secret_key` | ✅ | "请输入七牛 Secret Key" | 同上 |
   | `bucket` | ✅ | "Bucket 名称（默认 `rela-debug-log`）" | 给默认值时让用户能 Enter 接受 |
   | `domain` | ✅ | "CDN 域名（不含 `https://`）" | 写入时也不带 scheme |
   | `path_prefix` | ❌ | "文件路径前缀（留空则按 `{user_id}/` 搜索）" | 空字符串合法 |
   | `private` | ❌ | "是否私有空间？[Y/n]" | 默认 `true`，仅 `n / no` 视作 false |

   ### 2.2 写文件方式（务必）

   - ✅ 用 agent 自带的 **file-write 工具**直接写 YAML（Claude Code: `Write` 工具；其它 agent 类似）
   - ⛔ **不要**用 `printf "..." | qiniu-logs init` 之类 stdin pipe —— AK/SK 会落进 `ps`、会话日志、潜在的命令历史
   - ⛔ **不要**把 AK/SK 拼进 shell 命令的 argv（同上原因）
   - ⛔ **不要**自己猜 AK/SK 或从环境变量、其它配置文件里"借"

   ### 2.3 YAML 模板

   ```yaml
   qiniu:
     access_key: "<USER_AK>"
     secret_key: "<USER_SK>"
     bucket: "<USER_BUCKET>"
     domain: "<USER_DOMAIN>"
     path_prefix: "<USER_PATH_PREFIX 或空字符串>"
     use_https: true
     private: <true|false>
   ```

   ### 2.4 写完立刻收紧权限并验证

   ```bash
   chmod 700 ~/.qiniu-logs
   chmod 600 ~/.qiniu-logs/config.yaml
   qiniu-logs config
   ```

   `qiniu-logs config` 会脱敏只显示 AK/SK 前 4 位；看到 6 个字段非空即就绪。若报 `配置错误: X 不能为空`，回到 2.1 补齐相应字段。

   ### 2.5 退路：用户想自己交互填

   如果用户拒绝把 AK/SK 给 AI（合理的安全偏好），让用户自行运行交互式 CLI：
   ```bash
   qiniu-logs init
   ```
   等用户跑完，AI 再从 2 节开头重新检查 `~/.qiniu-logs/config.yaml`。

3. **拿到 `user_id`？**
   AI 不要凭空猜测 uid。如果用户描述里没有具体数字，回问一次。

## 可被 AI 驱动的命令（**非交互**）

| 意图 | 命令 |
|---|---|
| 最近 N 时长内的日志 | `qiniu-logs list <uid> --last 24h` |
| 指定日期范围 | `qiniu-logs list <uid> --from 2026-05-10 --to 2026-05-13` |
| 精确到时分秒 | `qiniu-logs list <uid> --from "2026-05-13 08:00:00" --to "2026-05-13 12:00:00"` |
| 限制返回条数 | `qiniu-logs list <uid> --last 24h --limit 10` |
| 按 key 下载到目录 | `qiniu-logs download <file_key> -o ./logs` |
| 用临时配置文件 | `qiniu-logs --config ./alt-config.yaml list <uid> --last 24h` |

### ⛔️ 禁用命令

- `qiniu-logs search ...` —— 进入 Bubble Tea TUI，需要键盘交互，**AI 不能驱动**。需要"挑选下载"场景时改走 `list` + `download` 的组合。

## 时间参数规则（与 CLI 实现一致）

- **绝对时间**支持 5 种布局，无时区按 `time.Local` 解析：
  - `2006-01-02`
  - `2006-01-02 15:04`
  - `2006-01-02 15:04:05`
  - `2006-01-02T15:04:05`
  - RFC3339（含偏移）
- **相对时长** `--last`：`30m / 24h / 7d / 1h30m`；`d` 会被展开成小时
- `--last` 与 `--from` **互斥**；`--to` 可与任一个组合
- `--to < --from` 会直接报错

## `list` 输出解析

每条文件占 **3 行**（v0.2.2+），第 3 行是裸的 `https://domain/key`——**不含签名 token**，仅作展示与识别用途。私有桶真正下载时由 `qiniu-logs download <key>` 在请求时刻就地签名。

```
时间范围: 2026-05-12 17:00:00 ~ (不限)
找到 3 个日志文件:

  1. logs/12345/app-2026-05-13.log
     大小: 4.20 MB | 时间: 2026-05-13 15:04:05
     URL:  https://cdn.example.com/logs/12345/app-2026-05-13.log
  2. logs/12345/err-2026-05-13.log
     大小: 18.40 KB | 时间: 2026-05-13 14:55:11
     URL:  https://cdn.example.com/logs/12345/err-2026-05-13.log
  3. logs/12345/app-2026-05-12.log
     大小: 3.81 MB | 时间: 2026-05-12 23:12:00
     URL:  https://cdn.example.com/logs/12345/app-2026-05-12.log

(私有空间：URL 直接打开会 401，请用 'qiniu-logs download <key>' 下载——会自动签名)
```

提取需要的字段：

```bash
# 提取 key（每条第 1 行的第 2 个字段）
qiniu-logs list 12345 --last 24h \
  | awk '/^[[:space:]]*[0-9]+\./ { print $2 }'

# 提取 URL（每条第 3 行）
qiniu-logs list 12345 --last 24h \
  | awk '/^[[:space:]]*URL:/ { print $2 }'

# key 与 URL 对齐成行
qiniu-logs list 12345 --last 24h \
  | awk '/^[[:space:]]*[0-9]+\./ { k=$2; next } /^[[:space:]]*URL:/ { print k"\t"$2 }'
```

注意：
- 首行 `找到 N 个日志文件` 给出**总数**，可 `head -1 | grep -oE '[0-9]+'` 抽取
- `时间范围` 行只在指定 `--from/--to/--last` 时出现
- 末尾私有空间提示行只在配置 `private: true` 时出现
- URL 是裸地址（无 `?e=...&token=...`），落到长期记忆/日志/PR 不会泄露下载凭证；私有桶 fetch 必须经 `qiniu-logs download <key>`
- 空结果输出 `未找到用户 <uid> 的日志文件`，据此向用户回退（uid 错？时间窗太窄？）

## 典型工作流

### A. "下载 user 12345 最近 24h 的日志"

```bash
mkdir -p ./logs
qiniu-logs list 12345 --last 24h \
  | awk '/^[[:space:]]*[0-9]+\./ { print $2 }' \
  | while read key; do
      qiniu-logs download "$key" -o ./logs
    done
ls -lh ./logs
```

### B. "查 user 12345 在 2026-05-13 的日志"

```bash
qiniu-logs list 12345 \
  --from "2026-05-13 00:00:00" \
  --to   "2026-05-13 23:59:59" \
  --limit 50
```

### C. "user 12345 在最近 7 天有多少条日志？"

```bash
qiniu-logs list 12345 --last 7d | head -1
```

### D. "把最新一条日志下载下来给我看"

```bash
key=$(qiniu-logs list 12345 --last 24h --limit 1 \
        | awk '/^[[:space:]]*[0-9]+\./ { print $2 }')
qiniu-logs download "$key" -o ./logs
```

## 故障处理速查

| 现象 | 处理 |
|---|---|
| `加载配置失败 ... 请先运行 'qiniu-logs init'` | 让用户交互跑 `qiniu-logs init` |
| `未找到用户 X 的日志文件` | 先确认 uid 是否正确；再放宽时间窗；最后检查 `path_prefix` 配置 |
| `下载失败，状态码: 401 / 403` | 私有桶签名 URL 鉴权失败；让用户重跑 `init` 检查 AK/SK 或 `private` 配置 |
| `下载失败，状态码: 404` | key 在桶中被清除，或 `domain` 配置错误 |
| `--last 与 --from 不能同时使用` | 二选一传给 CLI |

## 配置文件结构（参考，AI 不要自己改）

`~/.qiniu-logs/config.yaml`：

```yaml
qiniu:
  access_key: ""        # 七牛 AK
  secret_key: ""        # 七牛 SK
  bucket: rela-debug-log
  domain: ""            # CDN 域名，不含 https://
  path_prefix: ""       # 可选；若设置，搜索路径为 {path_prefix}/{user_id}/
  use_https: true
  private: true         # 私有空间会用签名 URL，1 小时有效
```

## 进一步资料

- README: https://github.com/Nevermore130/rela-qiniu-logs/blob/main/README.md
- 源码（列举逻辑）: `internal/qiniu/client.go`（`ListFiles` / `ListOptions`）
- 源码（时间解析）: `cmd/timerange.go`
