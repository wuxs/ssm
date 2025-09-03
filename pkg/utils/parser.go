// pkg/utils/parser.go
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseSSHHost 解析主机信息
func ParseSSHHost(host string) (username, hostname, port string) {
	// 解析 user@hostname:port 格式
	if atPos := strings.Index(host, "@"); atPos != -1 {
		username = host[:atPos]
		host = host[atPos+1:]
	}

	if colonPos := strings.Index(host, ":"); colonPos != -1 {
		hostname = host[:colonPos]
		port = host[colonPos+1:]
	} else {
		hostname = host
	}
	username = GetDefaultUsername(username)
	port = GetDefaultPort(port)

	return username, hostname, port
}

// GetDefaultUsername 获取默认用户名
func GetDefaultUsername(username string) string {
	if username != "" {
		return username
	}

	if user := os.Getenv("USER"); user != "" {
		return user
	}

	return "root"
}

// GetDefaultPort 获取默认端口
func GetDefaultPort(port string) string {
	if port != "" {
		return port
	}
	return "22"
}

// GetDefaultPrivateKeyPath 获取默认私钥路径
func GetDefaultPrivateKeyPath(privateKeyPath string) string {
	if privateKeyPath != "" {
		return privateKeyPath
	}
	return filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
}

func GetConfigKey(username, hostname, port string) string {
	key := fmt.Sprintf("%s@%s:%s", username, hostname, port)
	return key
}
