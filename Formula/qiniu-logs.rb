# Homebrew Formula for qiniu-logs
#
# 使用方法:
# 1. 创建自己的 Homebrew tap 仓库: homebrew-tap
# 2. 将此文件放入 Formula/ 目录
# 3. 用户安装: brew install your-username/tap/qiniu-logs
#
# 或者直接从本地安装:
# brew install --build-from-source ./Formula/qiniu-logs.rb

class QiniuLogs < Formula
  desc "七牛云日志文件下载工具 - 基于用户ID搜索和下载日志文件"
  homepage "https://github.com/Nevermore130/rela-qiniu-logs"
  version "0.1.3"
  license "MIT"

  # 根据实际发布地址修改
  on_macos do
    on_intel do
      url "https://github.com/Nevermore130/rela-qiniu-logs/releases/download/v#{version}/qiniu-logs_#{version}_darwin_amd64.tar.gz"
      # sha256 "填入实际的 SHA256 校验和"
    end

    on_arm do
      url "https://github.com/Nevermore130/rela-qiniu-logs/releases/download/v#{version}/qiniu-logs_#{version}_darwin_arm64.tar.gz"
      # sha256 "填入实际的 SHA256 校验和"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/Nevermore130/rela-qiniu-logs/releases/download/v#{version}/qiniu-logs_#{version}_linux_amd64.tar.gz"
      # sha256 "填入实际的 SHA256 校验和"
    end

    on_arm do
      url "https://github.com/Nevermore130/rela-qiniu-logs/releases/download/v#{version}/qiniu-logs_#{version}_linux_arm64.tar.gz"
      # sha256 "填入实际的 SHA256 校验和"
    end
  end

  def install
    bin.install "qiniu-logs"
  end

  def caveats
    <<~EOS
      首次使用前，请运行以下命令初始化配置:
        qiniu-logs init

      配置文件位置: ~/.qiniu-logs/config.yaml

      使用示例:
        qiniu-logs search 12345     # 交互式搜索用户日志
        qiniu-logs list 12345       # 列出用户日志文件
    EOS
  end

  test do
    assert_match "qiniu-logs version", shell_output("#{bin}/qiniu-logs --version")
  end
end
