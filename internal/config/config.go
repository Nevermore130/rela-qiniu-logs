package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Qiniu QiniuConfig `yaml:"qiniu"`
}

type QiniuConfig struct {
	AccessKey  string `yaml:"access_key"`
	SecretKey  string `yaml:"secret_key"`
	Bucket     string `yaml:"bucket"`
	Domain     string `yaml:"domain"`
	PathPrefix string `yaml:"path_prefix"`
	UseHTTPS   bool   `yaml:"use_https"`
	Private    bool   `yaml:"private"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".qiniu-logs/config.yaml"
	}
	return filepath.Join(home, ".qiniu-logs", "config.yaml")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Qiniu.AccessKey == "" {
		return fmt.Errorf("配置错误: access_key 不能为空")
	}
	if c.Qiniu.SecretKey == "" {
		return fmt.Errorf("配置错误: secret_key 不能为空")
	}
	if c.Qiniu.Bucket == "" {
		return fmt.Errorf("配置错误: bucket 不能为空")
	}
	if c.Qiniu.Domain == "" {
		return fmt.Errorf("配置错误: domain 不能为空")
	}
	return nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

func DefaultConfig() *Config {
	return &Config{
		Qiniu: QiniuConfig{
			AccessKey:  "",
			SecretKey:  "",
			Bucket:     "rela-debug-log",
			Domain:     "",
			PathPrefix: "",
			UseHTTPS:   true,
			Private:    true,
		},
	}
}
