package cmd

import (
	"github.com/rela/qiniu-logs/internal/config"
	"github.com/rela/qiniu-logs/internal/project"
)

// resolveProject picks the active project: explicit flag wins, else the
// config default. Returns a validated *project.Project.
func resolveProject(cfg *config.Config, flag string) (*project.Project, error) {
	name := flag
	if name == "" {
		name = cfg.Qiniu.DefaultProject
	}
	return cfg.Project(name)
}
