package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/qiniu"
)

var downloadCmd = &cobra.Command{
	Use:   "download <file_key>",
	Short: "直接下载指定的日志文件（非交互模式）",
	Long: `根据文件 key 直接下载日志文件，适合脚本使用。

示例:
  qiniu-logs download 12345/app.log
  qiniu-logs download 12345/app.log -o ./logs`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)
	downloadCmd.Flags().StringVarP(&outputDir, "output", "o", ".", "下载文件保存目录")
}

func runDownload(cmd *cobra.Command, args []string) error {
	fileKey := args[0]

	cfg, err := config.Load(getConfigPath())
	if err != nil {
		return fmt.Errorf("加载配置失败: %w\n\n请先运行 'qiniu-logs init' 初始化配置", err)
	}

	client := qiniu.NewClient(&cfg.Qiniu)

	filename := filepath.Base(fileKey)
	destPath := filepath.Join(outputDir, filename)

	fmt.Printf("正在下载: %s\n", fileKey)
	fmt.Printf("保存到: %s\n", destPath)

	lastPercent := -1
	err = client.DownloadFile(context.Background(), fileKey, destPath, func(downloaded, total int64) {
		if total > 0 {
			percent := int(float64(downloaded) / float64(total) * 100)
			if percent != lastPercent && percent%10 == 0 {
				fmt.Printf("进度: %d%%\n", percent)
				lastPercent = percent
			}
		}
	})

	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("下载失败: %w", err)
	}

	fmt.Printf("\n✓ 下载完成: %s\n", destPath)
	return nil
}
