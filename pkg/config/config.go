// pkg/config/config.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SSHConfig 表示SSH连接配置项
type SSHConfig struct {
	Host       string `json:"host"`
	Username   string `json:"username"`
	Port       string `json:"port"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
	ProxyJump  string `json:"-"`
	LastUsed   string `json:"last_used"`
}

// ConfigStore 表示SSH连接配置存储
type ConfigStore struct {
	Items map[string]SSHConfig `json:"items"`
}

// GetKey 获取配置的唯一键
func (c *SSHConfig) GetKey() string {
	return fmt.Sprintf("%s@%s:%s", c.Username, c.Host, c.Port)
}

// UpdateLastUsed 更新最后使用时间
func (c *SSHConfig) UpdateLastUsed() {
	c.LastUsed = time.Now().Format(time.RFC3339)
}

// GetAuthDescription 获取认证方式描述
func (c *SSHConfig) GetAuthDescription() string {
	authDesc := ""
	switch {
	case c.PrivateKey != "" && c.Password != "":
		authDesc = "password + key"
	case c.PrivateKey != "":
		authDesc = "key"
	case c.Password != "":
		authDesc = "password"
	default:
		authDesc = "none"
	}

	if c.ProxyJump != "" {
		authDesc += fmt.Sprintf(" + jump(%s)", c.ProxyJump)
	}

	return authDesc
}

// Load 加载配置
func Load() (*ConfigStore, error) {
	configFile := getConfigFilePath()
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return &ConfigStore{Items: make(map[string]SSHConfig)}, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var store ConfigStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	if store.Items == nil {
		store.Items = make(map[string]SSHConfig)
	}

	return &store, nil
}

// Save 保存配置
func Save(store *ConfigStore) error {
	configFile := getConfigFilePath()
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	return os.WriteFile(configFile, data, 0600)
}

// Get 获取配置
func Get(key string) (*SSHConfig, bool) {
	store, err := Load()
	if err != nil {
		return nil, false
	}

	config, exists := store.Items[key]
	if !exists {
		return nil, false
	}
	return &config, true
}

// SaveConfig 保存单个配置
func SaveConfig(config *SSHConfig) error {
	store, err := Load()
	if err != nil {
		return err
	}

	key := config.GetKey()
	store.Items[key] = *config

	return Save(store)
}

// Delete 删除配置
func Delete(key string) error {
	store, err := Load()
	if err != nil {
		return err
	}

	if _, exists := store.Items[key]; !exists {
		return fmt.Errorf("connection config not found: %s", key)
	}

	delete(store.Items, key)
	return Save(store)
}

// List 列出所有配置
func List() ([]SSHConfig, error) {
	store, err := Load()
	if err != nil {
		return nil, err
	}

	configs := make([]SSHConfig, 0, len(store.Items))
	for _, config := range store.Items {
		configs = append(configs, config)
	}

	// 按最后使用时间排序
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].LastUsed > configs[j].LastUsed
	})

	return configs, nil
}

// getConfigFilePath 获取配置文件路径
func getConfigFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}
	return filepath.Join(homeDir, ".ssm", "ssh_config.json")
}
