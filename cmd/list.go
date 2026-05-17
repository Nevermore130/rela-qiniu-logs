package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/qiniu"
	"github.com/spf13/cobra"
)

var (
	listLimit   int
	listFrom    string
	listTo      string
	listLast    string
	listProject string
)

var listCmd = &cobra.Command{
	Use:   "list <user_id>",
	Short: "列出指定用户的日志文件（非交互模式）",
	Long: `列出指定用户在七牛云存储中的所有日志文件，
以纯文本格式输出，适合脚本处理。

示例:
  qiniu-logs list 12345
  qiniu-logs list 12345 --limit 10
  qiniu-logs list 12345 --last 24h
  qiniu-logs list 12345 --from 2026-05-10 --to 2026-05-13
  qiniu-logs list 12345 --from "2026-05-13 08:00:00"`,
	Args: cobra.ExactArgs(1),
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 0, "限制显示数量 (0 表示不限制)")
	listCmd.Flags().StringVar(&listFrom, "from", "", "起始时间（含），支持 2006-01-02 / 2006-01-02 15:04:05 / RFC3339")
	listCmd.Flags().StringVar(&listTo, "to", "", "结束时间（含），格式同 --from")
	listCmd.Flags().StringVar(&listLast, "last", "", "最近时长（例如 30m / 24h / 7d / 1h30m），等价于 --from=now-<last>；与 --from 互斥")
	listCmd.Flags().StringVarP(&listProject, "project", "p", "", "项目名（不传则用配置中的 default_project）")
}

func runList(cmd *cobra.Command, args []string) error {
	userID := args[0]

	from, to, err := resolveTimeRange(listFrom, listTo, listLast, time.Now())
	if err != nil {
		return err
	}

	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	proj, err := resolveProject(cfg, listProject)
	if err != nil {
		return err
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	files, err := client.ListFiles(context.Background(), proj.ListPrefix(userID), proj.FileTime, qiniu.ListOptions{
		Limit: listLimit,
		From:  from,
		To:    to,
	})
	if err != nil {
		return fmt.Errorf("查询项目 %q 失败: %w", proj.Name, err)
	}

	if !from.IsZero() || !to.IsZero() {
		fmt.Printf("时间范围: %s ~ %s\n", formatBound(from, "(不限)"), formatBound(to, "(不限)"))
	}

	if len(files) == 0 {
		fmt.Printf("未找到用户 %s 的日志文件（项目: %s）\n", userID, proj.Name)
		return nil
	}

	fmt.Printf("找到 %d 个日志文件（项目: %s）:\n\n", len(files), proj.Name)
	for i, f := range files {
		fmt.Printf("%3d. %s\n", i+1, f.Key)
		fmt.Printf("     大小: %s | 时间: %s\n", qiniu.FormatSize(f.Size), f.LogTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("     URL:  %s\n", client.GetPublicURL(f.Key))
	}
	if cfg.Qiniu.Private {
		fmt.Println("\n(私有空间：URL 直接打开会 401，请用 'qiniu-logs download <key>' 下载——会自动签名)")
	}

	return nil
}
