package sshconfig

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wuxs/ssm/pkg/config"
	"github.com/wuxs/ssm/pkg/utils"
)

const (
	remarkPrefix   = "# SSM-Remark:"
	passwordPrefix = "# SSM-Password:"
)

type Options struct {
	IdentityFile string
	Port         string
	ProxyJump    string
}

type entry struct {
	Path     string
	Patterns []string
	Values   map[string]string
	Remark   string
	Password string
	Start    int
	End      int
}

type fileData struct {
	Path    string
	Lines   []string
	Entries []entry
}

func Resolve(target string, opts Options) (*config.SSHConfig, error) {
	user, alias, port := parseTarget(target)
	data, err := loadDefault()
	if err != nil {
		return nil, err
	}

	values := data.lookup(alias)
	hostname := firstNonEmpty(values["hostname"], alias)
	if user == "" {
		user = values["user"]
	}
	user = utils.GetDefaultUsername(user)
	if port == "" {
		port = values["port"]
	}
	if opts.Port != "" {
		port = opts.Port
	}
	port = utils.GetDefaultPort(port)

	privateKey := values["identityfile"]
	if opts.IdentityFile != "" {
		privateKey = opts.IdentityFile
	}
	proxyJump := values["proxyjump"]
	if opts.ProxyJump != "" {
		proxyJump = opts.ProxyJump
	}

	cfg := &config.SSHConfig{
		Alias:      alias,
		Host:       hostname,
		Username:   user,
		Port:       port,
		PrivateKey: expandPath(privateKey),
		ProxyJump:  proxyJump,
	}

	if exact := data.exactEntry(alias); exact != nil {
		cfg.Remark = exact.Remark
		cfg.Password = exact.Password
	}
	if cfg.Remark == "" && cfg.Alias != "" && cfg.Alias != cfg.Host {
		cfg.Remark = cfg.Alias
	}
	return cfg, nil
}

func List() ([]config.SSHConfig, error) {
	data, err := loadDefault()
	if err != nil {
		return nil, err
	}

	var configs []config.SSHConfig
	seen := map[string]bool{}
	for _, e := range data.Entries {
		if len(e.Patterns) == 0 || hasWildcardPattern(e.Patterns) {
			continue
		}
		for _, alias := range e.Patterns {
			if seen[alias] {
				continue
			}
			seen[alias] = true
			cfg, err := Resolve(alias, Options{})
			if err != nil {
				return nil, err
			}
			configs = append(configs, *cfg)
		}
	}

	Sort(configs)
	return configs, nil
}

func SaveRemark(cfg *config.SSHConfig) error {
	return updateMetadata(cfg.Alias, cfg.Remark, cfg.Password, true, false)
}

func SavePassword(cfg *config.SSHConfig) error {
	return updateMetadata(cfg.Alias, cfg.Remark, cfg.Password, false, true)
}

func Delete(key string) error {
	data, err := loadDefault()
	if err != nil {
		return err
	}
	if data.exactEntry(key) != nil {
		return data.deleteAlias(key)
	}
	return fmt.Errorf("ssh config host not found: %s", key)
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".ssh", "config")
}

func loadDefault() (*fileData, error) {
	path := ConfigPath()
	data, err := load(path)
	if os.IsNotExist(err) {
		return &fileData{Path: path}, nil
	}
	return data, err
}

func load(path string) (*fileData, error) {
	return loadFile(path, true, map[string]bool{})
}

func loadOwn(path string) (*fileData, error) {
	return loadFile(path, false, map[string]bool{})
}

func loadFile(path string, followIncludes bool, seen map[string]bool) (*fileData, error) {
	path = expandPath(path)
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	if seen[path] {
		return &fileData{Path: path}, nil
	}
	seen[path] = true

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := &fileData{Path: path}
	scanner := bufio.NewScanner(f)
	var current *entry
	lineNo := 0
	for scanner.Scan() {
		line := scanner.Text()
		data.Lines = append(data.Lines, line)
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if followIncludes && len(fields) > 1 && strings.EqualFold(fields[0], "Include") {
			included, err := loadIncludes(path, fields[1:], seen)
			if err != nil {
				return nil, err
			}
			data.Entries = append(data.Entries, included...)
		}
		if len(fields) > 0 && strings.EqualFold(fields[0], "Host") && len(fields) > 1 {
			if current != nil {
				current.End = lineNo
				data.Entries = append(data.Entries, *current)
			}
			current = &entry{Path: path, Patterns: fields[1:], Values: map[string]string{}, Start: lineNo}
			lineNo++
			continue
		}
		if current != nil {
			if strings.HasPrefix(trimmed, remarkPrefix) {
				current.Remark = strings.TrimSpace(strings.TrimPrefix(trimmed, remarkPrefix))
			} else if strings.HasPrefix(trimmed, passwordPrefix) {
				current.Password = decodePassword(strings.TrimSpace(strings.TrimPrefix(trimmed, passwordPrefix)))
			} else if len(fields) >= 2 {
				key := strings.ToLower(fields[0])
				switch key {
				case "hostname", "user", "port", "identityfile", "proxyjump":
					if _, exists := current.Values[key]; !exists {
						current.Values[key] = strings.Join(fields[1:], " ")
					}
				}
			}
		}
		lineNo++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		current.End = lineNo
		data.Entries = append(data.Entries, *current)
	}
	return data, nil
}

func loadIncludes(parentPath string, patterns []string, seen map[string]bool) ([]entry, error) {
	var entries []entry
	baseDir := filepath.Dir(parentPath)
	for _, pattern := range patterns {
		pattern = expandPath(pattern)
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(baseDir, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, match := range matches {
			included, err := loadFile(match, true, seen)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			entries = append(entries, included.Entries...)
		}
	}
	return entries, nil
}

func (d *fileData) lookup(alias string) map[string]string {
	values := map[string]string{}
	for _, e := range d.Entries {
		if !matchesAny(e.Patterns, alias) {
			continue
		}
		for key, value := range e.Values {
			if _, exists := values[key]; !exists {
				values[key] = value
			}
		}
	}
	return values
}

func (d *fileData) exactEntry(alias string) *entry {
	for i := range d.Entries {
		for _, pattern := range d.Entries[i].Patterns {
			if pattern == alias {
				return &d.Entries[i]
			}
		}
	}
	return nil
}

func (d *fileData) deleteAlias(alias string) error {
	idx := -1
	var target *entry
	for i, e := range d.Entries {
		for j, pattern := range e.Patterns {
			if pattern == alias {
				idx = j
				target = &d.Entries[i]
				break
			}
		}
		if idx >= 0 {
			break
		}
	}
	if target == nil {
		return fmt.Errorf("ssh config host not found: %s", alias)
	}

	own, err := loadOwn(target.Path)
	if err != nil {
		return err
	}
	e := own.exactEntry(alias)
	if e == nil {
		return fmt.Errorf("ssh config host not found in %s: %s", target.Path, alias)
	}
	lines := append([]string{}, own.Lines...)
	if len(e.Patterns) == 1 {
		lines = append(lines[:e.Start], lines[e.End:]...)
	} else {
		e.Patterns = append(e.Patterns[:idx], e.Patterns[idx+1:]...)
		lines[e.Start] = leadingIndent(lines[e.Start]) + "Host " + strings.Join(e.Patterns, " ")
	}
	return writeLines(own.Path, lines)
}

func updateMetadata(alias, remark, password string, updateRemark, updatePassword bool) error {
	data, err := loadDefault()
	if err != nil {
		return err
	}
	e := data.exactEntry(alias)
	if e == nil {
		return fmt.Errorf("ssh config host not found: %s", alias)
	}
	own, err := loadOwn(e.Path)
	if err != nil {
		return err
	}
	e = own.exactEntry(alias)
	if e == nil {
		return fmt.Errorf("ssh config host not found in %s: %s", own.Path, alias)
	}
	lines := append([]string{}, own.Lines...)
	insertAt := e.Start + 1
	remarkUpdated := !updateRemark
	passwordUpdated := !updatePassword
	for i := e.Start + 1; i < e.End; i++ {
		trimmed := strings.TrimSpace(lines[i])
		indent := leadingIndent(lines[i])
		if strings.HasPrefix(trimmed, remarkPrefix) && updateRemark {
			if remark == "" {
				lines = append(lines[:i], lines[i+1:]...)
				shiftEntryEnds(data.Entries, i, -1)
				e.End--
				i--
			} else {
				lines[i] = indent + remarkPrefix + " " + remark
			}
			remarkUpdated = true
			continue
		}
		if strings.HasPrefix(trimmed, passwordPrefix) && updatePassword {
			if password == "" {
				lines = append(lines[:i], lines[i+1:]...)
				shiftEntryEnds(data.Entries, i, -1)
				e.End--
				i--
			} else {
				lines[i] = indent + passwordPrefix + " " + encodePassword(password)
			}
			passwordUpdated = true
		}
	}
	indent := "  "
	if e.Start+1 < len(lines) {
		if got := leadingIndent(lines[e.Start+1]); got != "" {
			indent = got
		}
	}
	var inserts []string
	if updateRemark && !remarkUpdated && remark != "" {
		inserts = append(inserts, indent+remarkPrefix+" "+remark)
	}
	if updatePassword && !passwordUpdated && password != "" {
		inserts = append(inserts, indent+passwordPrefix+" "+encodePassword(password))
	}
	if len(inserts) > 0 {
		lines = append(lines[:insertAt], append(inserts, lines[insertAt:]...)...)
	}
	return writeLines(own.Path, lines)
}

func shiftEntryEnds(entries []entry, line, delta int) {
	for i := range entries {
		if entries[i].End > line {
			entries[i].End += delta
		}
	}
}

func writeLines(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0600)
}

func parseTarget(target string) (username, hostname, port string) {
	if atPos := strings.Index(target, "@"); atPos >= 0 {
		username = target[:atPos]
		target = target[atPos+1:]
	}
	if colonPos := strings.LastIndex(target, ":"); colonPos >= 0 && !strings.Contains(target[colonPos+1:], "]") {
		hostname = target[:colonPos]
		port = target[colonPos+1:]
	} else {
		hostname = target
	}
	return username, hostname, port
}

func matchesAny(patterns []string, alias string) bool {
	matched := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = strings.TrimPrefix(pattern, "!")
		}
		ok, err := filepath.Match(pattern, alias)
		if err != nil {
			ok = pattern == alias
		}
		if ok && negated {
			return false
		}
		if ok {
			matched = true
		}
	}
	return matched
}

func hasWildcardPattern(patterns []string) bool {
	for _, pattern := range patterns {
		if strings.ContainsAny(pattern, "*?!") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func expandPath(value string) string {
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(os.Getenv("HOME"), strings.TrimPrefix(value, "~/"))
	}
	return value
}

func leadingIndent(value string) string {
	return value[:len(value)-len(strings.TrimLeft(value, " \t"))]
}

func encodePassword(value string) string {
	return "base64:" + base64.StdEncoding.EncodeToString([]byte(value))
}

func decodePassword(value string) string {
	if strings.HasPrefix(value, "base64:") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "base64:"))
		if err == nil {
			return string(decoded)
		}
	}
	if strings.HasPrefix(value, "plain:") {
		return strings.TrimPrefix(value, "plain:")
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

func displayKey(cfg config.SSHConfig) string {
	if cfg.Alias != "" {
		return cfg.Alias
	}
	return cfg.GetKey()
}

func Sort(configs []config.SSHConfig) {
	sort.SliceStable(configs, func(i, j int) bool {
		if configs[i].LastUsed != configs[j].LastUsed {
			return configs[i].LastUsed > configs[j].LastUsed
		}
		return displayKey(configs[i]) < displayKey(configs[j])
	})
}
