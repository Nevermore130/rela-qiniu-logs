package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/rela/qiniu-logs/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "查看当前配置",
	Long:  `显示当前配置文件的内容（敏感信息已脱敏）`,
	RunE:  runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfgPath := getConfigPath()

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Printf("配置文件不存在: %s\n", cfgPath)
		fmt.Println("\n请运行 'qiniu-logs init' 初始化配置")
		return nil
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	fmt.Printf("配置文件: %s\n\n", cfgPath)
	fmt.Println("=== 当前配置 ===")
	fmt.Printf("Access Key: %s****\n", maskString(cfg.Qiniu.AccessKey, 4))
	fmt.Printf("Secret Key: %s****\n", maskString(cfg.Qiniu.SecretKey, 4))
	fmt.Printf("Bucket:     %s\n", cfg.Qiniu.Bucket)
	fmt.Printf("Domain:     %s\n", cfg.Qiniu.Domain)
	if cfg.Qiniu.PathPrefix != "" {
		fmt.Printf("PathPrefix: %s\n", cfg.Qiniu.PathPrefix)
	}
	fmt.Printf("UseHTTPS:   %t\n", cfg.Qiniu.UseHTTPS)
	fmt.Printf("Private:    %t\n", cfg.Qiniu.Private)
	fmt.Println()
	fmt.Printf("默认项目: %s\n", cfg.Qiniu.DefaultProject)
	fmt.Println("项目:")
	names := make([]string, 0, len(cfg.Qiniu.Projects))
	for n := range cfg.Qiniu.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := cfg.Qiniu.Projects[n]
		ts := p.TimeSource
		if ts == "" {
			ts = "put_time"
		}
		fmt.Printf("  %-16s prefix=%s  time=%s\n", n, p.Prefix, ts)
	}

	return nil
}

func maskString(s string, showLen int) string {
	if len(s) <= showLen {
		return s
	}
	return s[:showLen]
}
