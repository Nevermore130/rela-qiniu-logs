package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/qiniu"
	"github.com/rela/qiniu-logs/internal/ui"
)

var (
	outputDir string
)

var searchCmd = &cobra.Command{
	Use:   "search <user_id>",
	Short: "搜索并下载指定用户的日志文件",
	Long: `根据用户ID搜索七牛云存储中的日志文件，
并提供交互式界面选择要下载的文件。

示例:
  qiniu-logs search 12345
  qiniu-logs search 12345 -o ./logs`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "下载文件保存目录")
}

func runSearch(cmd *cobra.Command, args []string) error {
	userID := args[0]

	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	absOutput, err := os.Getwd()
	if err != nil {
		absOutput = outputDir
	} else if outputDir != "." {
		absOutput = outputDir
	}

	model := ui.NewModel(client, userID, absOutput)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("程序运行错误: %w", err)
	}

	return nil
}
