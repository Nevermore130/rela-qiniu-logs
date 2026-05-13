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
- PathPrefix: 文件路径前缀（可选）
- Private: 是否为私有空间

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

## 文件路径规则

工具会根据配置中的 `path_prefix` 和用户ID组合成完整的搜索路径：

- 无前缀时：搜索 `{user_id}/` 下的所有文件
- 有前缀时：搜索 `{path_prefix}/{user_id}/` 下的所有文件

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

### 安装（前提：已 `brew install qiniu-logs` 且跑过一次 `qiniu-logs init`）

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
