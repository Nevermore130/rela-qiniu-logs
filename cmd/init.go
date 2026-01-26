package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/rela/qiniu-logs/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化配置文件",
	Long: `创建或更新配置文件，交互式输入七牛云凭证和存储配置。

配置文件位置: ~/.qiniu-logs/config.yaml`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cfgPath := getConfigPath()
	reader := bufio.NewReader(os.Stdin)

	var existingCfg *config.Config
	if cfg, err := config.Load(cfgPath); err == nil {
		existingCfg = cfg
		fmt.Println("发现已有配置，按 Enter 保留原值")
		fmt.Println()
	}

	cfg := config.DefaultConfig()
	if existingCfg != nil {
		cfg = existingCfg
	}

	fmt.Println("=== 七牛云日志下载工具配置 ===")
	fmt.Println()

	cfg.Qiniu.AccessKey = promptWithDefault(reader, "Access Key", cfg.Qiniu.AccessKey, true)
	cfg.Qiniu.SecretKey = promptWithDefault(reader, "Secret Key", cfg.Qiniu.SecretKey, true)
	cfg.Qiniu.Bucket = promptWithDefault(reader, "Bucket 名称", cfg.Qiniu.Bucket, false)
	cfg.Qiniu.Domain = promptWithDefault(reader, "CDN 域名 (不含 https://)", cfg.Qiniu.Domain, false)
	cfg.Qiniu.PathPrefix = promptWithDefault(reader, "文件路径前缀 (可选，留空表示无前缀)", cfg.Qiniu.PathPrefix, false)

	privateStr := "y"
	if !cfg.Qiniu.Private {
		privateStr = "n"
	}
	privateInput := promptWithDefault(reader, "是否为私有空间 (y/n)", privateStr, false)
	cfg.Qiniu.Private = strings.ToLower(privateInput) == "y" || strings.ToLower(privateInput) == "yes"

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Println()
	fmt.Printf("✓ 配置已保存到: %s\n", cfgPath)
	fmt.Println()
	fmt.Println("现在可以使用 'qiniu-logs search <user_id>' 搜索日志文件")

	return nil
}

func promptWithDefault(reader *bufio.Reader, prompt string, defaultVal string, sensitive bool) string {
	displayDefault := defaultVal
	if sensitive && len(defaultVal) > 4 {
		displayDefault = defaultVal[:4] + "****"
	}

	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", prompt, displayDefault)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}
