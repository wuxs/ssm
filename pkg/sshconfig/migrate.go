package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wuxs/ssm/pkg/config"
)

const ssmConfigName = "ssm.config"

type MigrationResult struct {
	MigratedCount int
	SSMConfigPath string
	SSHConfigPath string
	IncludeAdded  bool
}

func SaveHost(cfg *config.SSHConfig) error {
	ssmPath := SSMConfigPath()
	existing, err := loadSSMConfigEntries(ssmPath)
	if err != nil {
		return err
	}
	alias := migrationAlias(*cfg)
	if alias == "" {
		alias = cfg.Host
	}
	if alias == "" {
		alias = cfg.GetKey()
	}
	next := *cfg
	next.Alias = alias
	if next.Remark == "" && alias != next.Host {
		next.Remark = alias
	}
	replaced := false
	for i := range existing {
		if existing[i].Alias == next.Alias || migrationIdentity(existing[i]) == migrationIdentity(next) {
			existing[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, next)
	}
	if err := writeSSMConfig(ssmPath, existing); err != nil {
		return err
	}
	_, err = ensureSSMInclude(ConfigPath(), ssmPath)
	return err
}

func SSMConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".ssh", ssmConfigName)
}

func MigrateLegacy() (*MigrationResult, error) {
	ssmPath := SSMConfigPath()
	existing, err := loadSSMConfigEntries(ssmPath)
	if err != nil {
		return nil, err
	}
	if err := writeSSMConfig(ssmPath, existing); err != nil {
		return nil, err
	}
	includeAdded, err := ensureSSMInclude(ConfigPath(), ssmPath)
	if err != nil {
		return nil, err
	}
	return &MigrationResult{
		MigratedCount: len(existing),
		SSMConfigPath: ssmPath,
		SSHConfigPath: ConfigPath(),
		IncludeAdded:  includeAdded,
	}, nil
}

func migrationIdentity(cfg config.SSHConfig) string {
	if cfg.Host == "" {
		return ""
	}
	return cfg.Username + "@" + cfg.Host + ":" + cfg.Port
}

func loadSSMConfigEntries(path string) ([]config.SSHConfig, error) {
	data, err := loadOwn(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	configs := make([]config.SSHConfig, 0, len(data.Entries))
	for _, e := range data.Entries {
		if len(e.Patterns) == 0 || hasWildcardPattern(e.Patterns) {
			continue
		}
		for _, alias := range e.Patterns {
			host := firstNonEmpty(e.Values["hostname"], alias)
			remark := e.Remark
			if remark == "" && alias != host {
				remark = alias
			}
			configs = append(configs, config.SSHConfig{
				Alias:      alias,
				Remark:     remark,
				Host:       host,
				Username:   e.Values["user"],
				Port:       e.Values["port"],
				PrivateKey: e.Values["identityfile"],
				Password:   e.Password,
				ProxyJump:  e.Values["proxyjump"],
			})
		}
	}
	return configs, nil
}

func writeSSMConfig(path string, configs []config.SSHConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	sort.SliceStable(configs, func(i, j int) bool {
		return configs[i].GetKey() < configs[j].GetKey()
	})
	var b strings.Builder
	b.WriteString("# Managed by ssm migrate from ~/.ssh/ssm.config\n")
	b.WriteString("# You can edit this file manually; OpenSSH ignores SSM comment metadata.\n\n")
	for _, cfg := range configs {
		alias := migrationAlias(cfg)
		if alias == "" {
			alias = cfg.GetKey()
		}
		b.WriteString(fmt.Sprintf("Host %s\n", alias))
		if cfg.Remark != "" && cfg.Remark != alias {
			b.WriteString(fmt.Sprintf("  %s %s\n", remarkPrefix, cfg.Remark))
		}
		if cfg.Password != "" {
			b.WriteString(fmt.Sprintf("  %s %s\n", passwordPrefix, encodePassword(cfg.Password)))
		}
		if cfg.Host != "" {
			b.WriteString(fmt.Sprintf("  HostName %s\n", cfg.Host))
		}
		if cfg.Username != "" {
			b.WriteString(fmt.Sprintf("  User %s\n", cfg.Username))
		}
		if cfg.Port != "" {
			b.WriteString(fmt.Sprintf("  Port %s\n", cfg.Port))
		}
		if cfg.PrivateKey != "" {
			b.WriteString(fmt.Sprintf("  IdentityFile %s\n", cfg.PrivateKey))
		}
		if cfg.ProxyJump != "" {
			b.WriteString(fmt.Sprintf("  ProxyJump %s\n", cfg.ProxyJump))
		}
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}

func migrationAlias(cfg config.SSHConfig) string {
	alias := cfg.Host
	if cfg.Alias != "" {
		alias = cfg.Alias
	}
	if cfg.Remark != "" && isSingleHostToken(cfg.Remark) && (cfg.Alias == "" || cfg.Alias == cfg.Host) {
		alias = cfg.Remark
	}
	return alias
}

func isSingleHostToken(value string) bool {
	return len(strings.Fields(value)) == 1
}

func ensureSSMInclude(configPath, ssmPath string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return false, err
	}
	includeLine := "Include ~/.ssh/" + ssmConfigName
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return true, os.WriteFile(configPath, []byte(includeLine+"\n"), 0600)
	}
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, includeLine) || includeMatchesPath(trimmed, ssmPath) {
			return false, nil
		}
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += includeLine + "\n"
	return true, os.WriteFile(configPath, []byte(content), 0600)
}

func includeMatchesPath(line, ssmPath string) bool {
	fields := strings.Fields(line)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Include") {
		return false
	}
	pattern := expandPath(fields[1])
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(ConfigPath()), pattern)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false
	}
	ssmAbs, err := filepath.Abs(ssmPath)
	if err == nil {
		ssmPath = ssmAbs
	}
	for _, match := range matches {
		matchAbs, err := filepath.Abs(match)
		if err == nil {
			match = matchAbs
		}
		if match == ssmPath {
			return true
		}
	}
	return false
}
