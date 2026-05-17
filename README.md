# qiniu-logs

七牛云日志文件下载工具 - 基于用户ID搜索和下载日志文件的命令行工具。

## 功能特点

- 基于用户ID搜索七牛云存储中的日志文件
- 交互式文件选择界面，支持键盘导航和搜索过滤
- 支持私有空间签名下载
- 显示下载进度
- 支持脚本批量操作（非交互模式）
- 易于通过 Homebrew 安装

## 安装

### 通过 Homebrew（推荐）

```bash
# 添加 tap 并安装
brew tap Nevermore130/rela-qiniu-logs
brew install qiniu-logs

# 或一行命令安装
brew install Nevermore130/rela-qiniu-logs/qiniu-logs
```

### 下载预编译版本

从 [Releases](https://github.com/Nevermore130/rela-qiniu-logs/releases) 页面下载适合您系统的版本：

```bash
# macOS (Apple Silicon)
curl -LO https://github.com/Nevermore130/rela-qiniu-logs/releases/latest/download/qiniu-logs_darwin_arm64.tar.gz
tar -xzf qiniu-logs_darwin_arm64.tar.gz
sudo mv qiniu-logs /usr/local/bin/

# macOS (Intel)
curl -LO https://github.com/Nevermore130/rela-qiniu-logs/releases/latest/download/qiniu-logs_darwin_amd64.tar.gz
tar -xzf qiniu-logs_darwin_amd64.tar.gz
sudo mv qiniu-logs /usr/local/bin/

# Linux (AMD64)
curl -LO https://github.com/Nevermore130/rela-qiniu-logs/releases/latest/download/qiniu-logs_linux_amd64.tar.gz
tar -xzf qiniu-logs_linux_amd64.tar.gz
sudo mv qiniu-logs /usr/local/bin/
```

### 从源码安装

```bash
git clone https://github.com/Nevermore130/rela-qiniu-logs.git
cd rela-qiniu-logs
make build
sudo mv qiniu-logs /usr/local/bin/
```

## 配置

首次使用前，需要初始化配置：

```bash
qiniu-logs init
```

按提示输入：
- Access Key: 七牛云 Access Key
- Secret Key: 七牛云 Secret Key
- Bucket: 存储空间名称（如 `rela-debug-log`）
- Domain: CDN 域名（不含 https://）
- Private: 是否为私有空间
- `projects`: 各产品的路径布局（prefix 模板 + 时间来源），在 `config.yaml` 的 `projects:` 下声明
- `default_project`: 不传 `--project` 时使用的项目名称

> `path_prefix` 仍受支持（旧版遗留字段），会自动合成一个 `default` 项目，行为不变。新配置建议直接使用 `projects` 映射。

配置文件保存在 `~/.qiniu-logs/config.yaml`

## 使用

### 交互式搜索（推荐）

```bash
# 搜索用户 12345 的日志文件
qiniu-logs search 12345

# 指定下载目录
qiniu-logs search 12345 -o ./logs
```

使用方向键选择文件，按 Enter 下载，按 `/` 搜索过滤。

### 列出文件（非交互）

```bash
# 列出用户日志文件
qiniu-logs list 12345

# 限制显示数量
qiniu-logs list 12345 -n 10
```

### 直接下载

```bash
# 根据文件 key 直接下载
qiniu-logs download 12345/app.log -o ./logs
```

### 查看配置

```bash
qiniu-logs config
```

## 命令参考

| 命令 | 说明 |
|------|------|
| `init` | 初始化配置文件 |
| `search <user_id>` | 交互式搜索和下载 |
| `list <user_id>` | 列出用户日志文件 |
| `download <file_key>` | 直接下载文件 |
| `config` | 查看当前配置 |

### 全局选项

| 选项 | 说明 |
|------|------|
| `--config` | 指定配置文件路径 |
| `--version` | 显示版本号 |
| `--help` | 显示帮助信息 |

### list / search 专属选项

`list` 和 `search` 支持 `-p, --project <name>` 选择项目（默认使用配置中的 `default_project`）。例如：

```bash
qiniu-logs list 12345 --project live_service --last 24h
qiniu-logs search 12345 --project live_service
```

## 文件路径规则

工具按「项目」决定搜索路径。每个项目在 config.yaml 的 `projects:` 下声明：

- `prefix`: 含 `{uid}` 占位符的前缀模板，例如 `{uid}`（默认）或 `live_service/{uid}/`
- `time_source`: `put_time`（按对象上传时间，默认）或 `path`（从 key 解析时间）
- `time_regex` / `time_layout`: 当 `time_source: path` 时，用恰好一个捕获组的正则
  抓出时间子串，再用 Go 时间布局解析

查询时用 `--project <name>` 选择；不传则用 `default_project`。
旧配置（只有 `path_prefix`）会自动合成一个 `default` 项目，行为不变。

示例（直播小助手独立路径）：
`live_service/12345/20260516_1030/log_12345_20260516_103015_a1b2c3d4.zip`
对应项目 `live_service`：prefix `live_service/{uid}/`，time_source `path`，
time_regex `_(\d{8}_\d{6})_`，time_layout `20060102_150405`。

## 开发

```bash
# 下载依赖
make deps

# 构建
make build

# 运行测试
make test

# 交叉编译所有平台
make release
```

## 发布

使用 GoReleaser 自动发布：

```bash
# 打标签并推送
git tag v0.1.4
git push origin v0.1.4
```

GitHub Actions 会自动：
1. 构建多平台二进制文件（macOS/Linux/Windows）
2. 上传到 GitHub Releases
3. 更新 Homebrew Formula

## AI / Claude Code Skill

本仓库自带一个 Claude Code skill（`skill/SKILL.md`），装载之后 AI agent 会在你说「下载用户 X 最近 24h 的日志」这类需求时，自动知道用本工具并按 `list → download` 流程跑。

> **关于 CLI 自身**：AI agent（Claude Code / Codex / Qoder 等）读到 SKILL.md 后，如果检测到 `qiniu-logs` 未安装，会**自动执行 `brew install Nevermore130/rela-qiniu-logs/qiniu-logs`**（含无 brew 的 `~/.local/bin` 后备路径）。SKILL.md 已经写明 brew 不需要 sudo，安装是低风险动作；但仍按各 agent 的"破坏性动作前告知"约定通告一句。
> 配置初始化（AK/SK/domain/bucket）改由 **AI 用自己的提问 UI 逐项问你**，再用文件写入工具直接落 `~/.qiniu-logs/config.yaml`；这比 `qiniu-logs init` + stdin pipe 安全（不会让 AK/SK 出现在 `ps` 或会话日志里）。如果你不愿把 AK/SK 给 AI，可手动跑 `qiniu-logs init` 自填，AI 会在配置就绪后继续。

### 安装（前提：CLI 已就绪 —— 你装好 brew + qiniu-logs，或让 AI 跑 SKILL 的 install 段落帮你装）

```bash
git clone https://github.com/Nevermore130/rela-qiniu-logs.git  # 已 clone 可跳过
cd rela-qiniu-logs
make install-skill     # 会把 skill/ 符号链接到 ~/.claude/skills/qiniu-logs/
```

打开新的 Claude Code 会话即可生效。卸载：

```bash
make uninstall-skill
```

### Skill 教给 AI 的能力范围

- ✅ `qiniu-logs list <uid> --last 24h / --from / --to / --limit`
- ✅ `qiniu-logs download <file_key> -o ./logs`
- ✅ 输出解析、常见工作流、故障处理
- ⛔ 不驱动 `qiniu-logs search`（TUI 模式，AI 无法操作键盘）
- ⛔ 不替代用户输入 AK/SK；`qiniu-logs init` 始终由用户交互完成

更多细节直接读 [`skill/SKILL.md`](./skill/SKILL.md)。

## 许可证

MIT License
