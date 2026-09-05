package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	Deployments map[string]deployment `yaml:"deployments"`
	Plans       map[string]plan       `yaml:"plans"`
}

type deployment struct {
	Uploads     []upload     `yaml:"uploads"`
	Activations []activation `yaml:"activations"`
}

type plan struct {
	Deployments map[string]planDeployment `yaml:"deployments"`
}

type planDeployment struct {
	Needs []string `yaml:"needs"`
}

type upload struct {
	Label    string   `yaml:"label"`
	Role     string   `yaml:"role"`
	Local    string   `yaml:"local"`
	Remote   string   `yaml:"remote"`
	Delete   bool     `yaml:"delete"`
	Excludes []string `yaml:"excludes"`
	Validate string   `yaml:"validate"`
}

type activation struct {
	Label  string `yaml:"label"`
	Role   string `yaml:"role"`
	Script string `yaml:"script"`
}

var (
	placeholderRE = regexp.MustCompile(`\{(host|var:[A-Za-z0-9_]+)\}`)
	nameRE        = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	hostRE        = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	valueRE       = regexp.MustCompile(`^[A-Za-z0-9_./,:-]+$`)
	remotePathRE  = regexp.MustCompile(`^/[A-Za-z0-9_./-]+/?$`)
)

func loadConfig(filename string) (*config, error) {
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("デプロイ宣言を読めません: %w", err)
	}
	var cfg config
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("デプロイ宣言を解釈できません: %w", err)
	}
	if len(cfg.Deployments) == 0 {
		return nil, fmt.Errorf("%s に deployments がありません", filename)
	}
	for name, d := range cfg.Deployments {
		if !nameRE.MatchString(name) {
			return nil, fmt.Errorf("不正な deployment 名です: %q", name)
		}
		if len(d.Uploads) == 0 && len(d.Activations) == 0 {
			return nil, fmt.Errorf("deployment %q には uploads または activations が必要です", name)
		}
		for _, u := range d.Uploads {
			if u.Label == "" || u.Role == "" || u.Local == "" || u.Remote == "" {
				return nil, fmt.Errorf("deployment %q の upload に label / role / local / remote のいずれかが足りません", name)
			}
			if err := validateLocalPath(u.Local); err != nil {
				return nil, fmt.Errorf("deployment %q upload %q: %w", name, u.Label, err)
			}
			if u.Delete && (!strings.HasSuffix(u.Local, "/") || !strings.HasSuffix(u.Remote, "/")) {
				return nil, fmt.Errorf("deployment %q upload %q: delete は末尾 / のディレクトリ同士にだけ指定できます", name, u.Label)
			}
		}
		for _, a := range d.Activations {
			if a.Label == "" || a.Role == "" || strings.TrimSpace(a.Script) == "" {
				return nil, fmt.Errorf("deployment %q の activation に label / role / script のいずれかが足りません", name)
			}
		}
	}
	for name, p := range cfg.Plans {
		if !nameRE.MatchString(name) {
			return nil, fmt.Errorf("不正な plan 名です: %q", name)
		}
		if len(p.Deployments) == 0 {
			return nil, fmt.Errorf("plan %q には deployments が必要です", name)
		}
		for deploymentName, node := range p.Deployments {
			if _, ok := cfg.Deployments[deploymentName]; !ok {
				return nil, fmt.Errorf("plan %q が未知の deployment %q を参照しています", name, deploymentName)
			}
			for _, dependency := range node.Needs {
				if _, ok := p.Deployments[dependency]; !ok {
					return nil, fmt.Errorf("plan %q deployment %q が plan 内にない dependency %q を参照しています", name, deploymentName, dependency)
				}
			}
		}
		if err := validatePlanAcyclic(name, p); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func validatePlanAcyclic(name string, p plan) error {
	const (
		unvisited = iota
		visiting
		visited
	)
	states := make(map[string]int, len(p.Deployments))
	var visit func(string) error
	visit = func(deploymentName string) error {
		switch states[deploymentName] {
		case visiting:
			return fmt.Errorf("plan %q の依存関係に循環があります (%s)", name, deploymentName)
		case visited:
			return nil
		}
		states[deploymentName] = visiting
		for _, dependency := range p.Deployments[deploymentName].Needs {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		states[deploymentName] = visited
		return nil
	}
	for deploymentName := range p.Deployments {
		if err := visit(deploymentName); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalPath(value string) error {
	expanded := placeholderRE.ReplaceAllString(value, "placeholder")
	clean := filepath.Clean(expanded)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("local path はリポジトリ内の相対パスにしてください: %q", value)
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "webapp" && parts[i+1] == "node" {
			return fmt.Errorf("webapp/node はデプロイできません")
		}
	}
	return nil
}

type expander struct {
	host string
	vars map[string]string
}

func (e expander) expand(value string) (string, error) {
	missing := make([]string, 0)
	expanded := placeholderRE.ReplaceAllStringFunc(value, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{var:"), "}")
		if match == "{host}" {
			if e.host == "" {
				missing = append(missing, match)
				return match
			}
			return e.host
		}
		v, ok := e.vars[key]
		if !ok {
			missing = append(missing, match)
			return match
		}
		return v
	})
	if len(missing) != 0 {
		return "", fmt.Errorf("未指定のプレースホルダがあります: %s", strings.Join(missing, ", "))
	}
	if strings.Contains(expanded, "{var:") || strings.Contains(expanded, "{host}") {
		return "", fmt.Errorf("解釈できないプレースホルダがあります: %q", expanded)
	}
	return expanded, nil
}
