package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	cfgFile string
)

var rootCmd = &cobra.Command{
	Use:   "qiniu-logs",
	Short: "七牛云日志文件下载工具",
	Long: `qiniu-logs 是一个用于从七牛云对象存储下载用户日志文件的命令行工具。

使用示例:
  # 搜索并下载指定用户的日志文件
  qiniu-logs search 12345

  # 初始化配置文件
  qiniu-logs init

  # 查看配置
  qiniu-logs config`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认: ~/.qiniu-logs/config.yaml)")

	rootCmd.SetVersionTemplate(fmt.Sprintf("qiniu-logs version %s\n", version))
}

func getConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".qiniu-logs/config.yaml"
	}
	return fmt.Sprintf("%s/.qiniu-logs/config.yaml", home)
}
