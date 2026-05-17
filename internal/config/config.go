package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rela/qiniu-logs/internal/project"
)

type Config struct {
	Qiniu QiniuConfig `yaml:"qiniu"`
}

type ProjectConfig struct {
	Prefix     string `yaml:"prefix"`
	TimeSource string `yaml:"time_source"`
	TimeRegex  string `yaml:"time_regex"`
	TimeLayout string `yaml:"time_layout"`
}

type QiniuConfig struct {
	AccessKey      string                   `yaml:"access_key"`
	SecretKey      string                   `yaml:"secret_key"`
	Bucket         string                   `yaml:"bucket"`
	Domain         string                   `yaml:"domain"`
	PathPrefix     string                   `yaml:"path_prefix,omitempty"` // legacy; only for backward-compat synthesis
	UseHTTPS       bool                     `yaml:"use_https"`
	Private        bool                     `yaml:"private"`
	DefaultProject string                   `yaml:"default_project"`
	Projects       map[string]ProjectConfig `yaml:"projects"`
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

	cfg.synthesizeDefaultProject()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) synthesizeDefaultProject() {
	if len(c.Qiniu.Projects) > 0 {
		return
	}
	prefix := "{uid}"
	if c.Qiniu.PathPrefix != "" {
		prefix = c.Qiniu.PathPrefix + "/{uid}"
	}
	c.Qiniu.Projects = map[string]ProjectConfig{
		"default": {Prefix: prefix, TimeSource: string(project.TimePutTime)},
	}
	if c.Qiniu.DefaultProject == "" {
		c.Qiniu.DefaultProject = "default"
	}
	// Additively expose live_service alongside the synthesized default.
	// The !ok guard is defensive: harmless today, future-proof if another
	// synthesis branch ever pre-populates the key.
	if _, ok := c.Qiniu.Projects["live_service"]; !ok {
		c.Qiniu.Projects["live_service"] = builtinProjects()["live_service"]
	}
}

// Project builds a validated *project.Project for the given name.
func (c *Config) Project(name string) (*project.Project, error) {
	pc, ok := c.Qiniu.Projects[name]
	if !ok {
		return nil, fmt.Errorf("未知项目 %q；可用项目: %s", name, c.projectNames())
	}
	p := projectFromConfig(name, pc)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func projectFromConfig(name string, pc ProjectConfig) *project.Project {
	return &project.Project{
		Name:       name,
		Prefix:     pc.Prefix,
		TimeSource: project.TimeSource(pc.TimeSource),
		TimeRegex:  pc.TimeRegex,
		TimeLayout: pc.TimeLayout,
	}
}

func (c *Config) projectNames() string {
	names := make([]string, 0, len(c.Qiniu.Projects))
	for n := range c.Qiniu.Projects {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
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
	if len(c.Qiniu.Projects) > 0 && c.Qiniu.DefaultProject == "" {
		return fmt.Errorf("配置错误: projects 非空时 default_project 不能为空")
	}
	if c.Qiniu.DefaultProject != "" {
		if _, ok := c.Qiniu.Projects[c.Qiniu.DefaultProject]; !ok {
			return fmt.Errorf("配置错误: default_project %q 不在 projects 中", c.Qiniu.DefaultProject)
		}
	}
	for name, pc := range c.Qiniu.Projects {
		p := projectFromConfig(name, pc)
		if err := p.Validate(); err != nil {
			return fmt.Errorf("配置错误: %w", err)
		}
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

// builtinProjects returns the projects shipped preconfigured: a plain
// uid-prefixed project plus the live_service path-timestamp layout.
func builtinProjects() map[string]ProjectConfig {
	return map[string]ProjectConfig{
		"rela-debug-log": {
			Prefix:     "{uid}",
			TimeSource: string(project.TimePutTime),
		},
		"live_service": {
			Prefix:     "live_service/{uid}/",
			TimeSource: string(project.TimePath),
			TimeRegex:  `_(\d{8}_\d{6})_`,
			TimeLayout: "20060102_150405",
		},
	}
}

func DefaultConfig() *Config {
	return &Config{
		Qiniu: QiniuConfig{
			AccessKey:      "",
			SecretKey:      "",
			Bucket:         "rela-debug-log",
			Domain:         "",
			UseHTTPS:       true,
			Private:        true,
			DefaultProject: "rela-debug-log",
			Projects:       builtinProjects(),
		},
	}
}
