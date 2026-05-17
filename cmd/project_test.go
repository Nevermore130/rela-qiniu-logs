package cmd

import (
	"testing"

	"github.com/rela/qiniu-logs/internal/config"
)

func cfgWith(def string, names ...string) *config.Config {
	pc := map[string]config.ProjectConfig{}
	for _, n := range names {
		pc[n] = config.ProjectConfig{Prefix: "{uid}", TimeSource: "put_time"}
	}
	return &config.Config{Qiniu: config.QiniuConfig{DefaultProject: def, Projects: pc}}
}

func TestResolveProjectFlagWins(t *testing.T) {
	c := cfgWith("a", "a", "b")
	p, err := resolveProject(c, "b")
	if err != nil || p.Name != "b" {
		t.Fatalf("got (%v, %v), want project b", p, err)
	}
}

func TestResolveProjectFallsBackToDefault(t *testing.T) {
	c := cfgWith("a", "a", "b")
	p, err := resolveProject(c, "")
	if err != nil || p.Name != "a" {
		t.Fatalf("got (%v, %v), want default project a", p, err)
	}
}

func TestResolveProjectUnknownErrors(t *testing.T) {
	c := cfgWith("a", "a")
	if _, err := resolveProject(c, "ghost"); err == nil {
		t.Fatal("expected error for unknown project")
	}
}
