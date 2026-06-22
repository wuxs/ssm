package config

import (
	"fmt"
	"time"
)

// SSHConfig 表示SSH连接配置项
type SSHConfig struct {
	Alias      string `json:"-"`
	Remark     string `json:"remark,omitempty"`
	Host       string `json:"host"`
	Username   string `json:"username"`
	Port       string `json:"port"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
	ProxyJump  string `json:"-"`
	LastUsed   string `json:"last_used"`
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
