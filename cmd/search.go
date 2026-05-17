package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/qiniu"
	"github.com/rela/qiniu-logs/internal/ui"
	"github.com/spf13/cobra"
)

var (
	outputDir     string
	searchFrom    string
	searchTo      string
	searchLast    string
	searchProject string
)

var searchCmd = &cobra.Command{
	Use:   "search <user_id>",
	Short: "搜索并下载指定用户的日志文件",
	Long: `根据用户ID搜索七牛云存储中的日志文件，
并提供交互式界面选择要下载的文件。

示例:
  qiniu-logs search 12345
  qiniu-logs search 12345 -o ./logs
  qiniu-logs search 12345 --last 24h
  qiniu-logs search 12345 --from 2026-05-10 --to 2026-05-13`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "下载文件保存目录")
	searchCmd.Flags().StringVar(&searchFrom, "from", "", "起始时间（含），支持 2006-01-02 / 2006-01-02 15:04:05 / RFC3339")
	searchCmd.Flags().StringVar(&searchTo, "to", "", "结束时间（含），格式同 --from")
	searchCmd.Flags().StringVar(&searchLast, "last", "", "最近时长（例如 30m / 24h / 7d / 1h30m），与 --from 互斥")
	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "项目名（不传则用配置中的 default_project）")
}

func runSearch(cmd *cobra.Command, args []string) error {
	userID := args[0]

	from, to, err := resolveTimeRange(searchFrom, searchTo, searchLast, time.Now())
	if err != nil {
		return err
	}

	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	proj, err := resolveProject(cfg, searchProject)
	if err != nil {
		return err
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	absOutput, err := os.Getwd()
	if err != nil {
		absOutput = outputDir
	} else if outputDir != "." {
		absOutput = outputDir
	}

	model := ui.NewModel(client, userID, proj.ListPrefix(userID), proj.FileTime, absOutput, qiniu.ListOptions{From: from, To: to})
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("程序运行错误: %w", err)
	}

	return nil
}
