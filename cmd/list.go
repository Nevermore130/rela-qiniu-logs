package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/qiniu"
)

var (
	listLimit int
)

var listCmd = &cobra.Command{
	Use:   "list <user_id>",
	Short: "列出指定用户的日志文件（非交互模式）",
	Long: `列出指定用户在七牛云存储中的所有日志文件，
以纯文本格式输出，适合脚本处理。

示例:
  qiniu-logs list 12345
  qiniu-logs list 12345 --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 0, "限制显示数量 (0 表示不限制)")
}

func runList(cmd *cobra.Command, args []string) error {
	userID := args[0]

	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	files, err := client.ListFiles(context.Background(), userID, listLimit)
	if err != nil {
		return fmt.Errorf("列举文件失败: %w", err)
	}

	if len(files) == 0 {
		fmt.Printf("未找到用户 %s 的日志文件\n", userID)
		return nil
	}

	fmt.Printf("找到 %d 个日志文件:\n\n", len(files))
	for i, f := range files {
		fmt.Printf("%3d. %s\n", i+1, f.Key)
		fmt.Printf("     大小: %s | 时间: %s\n", qiniu.FormatSize(f.Size), f.PutTime.Format("2006-01-02 15:04:05"))
	}

	return nil
}
