// pkg/auth/auth.go
package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wuxs/ssm/pkg/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// CreateClientConfig 创建SSH客户端配置，按照标准SSH认证顺序
func CreateClientConfig(cfg *config.SSHConfig) (*ssh.ClientConfig, error) {
	var authMethods []ssh.AuthMethod

	// 1. 尝试公钥认证
	if signers := getAvailableSigners(cfg.PrivateKey); len(signers) > 0 {
		authMethods = append(authMethods, ssh.PublicKeys(signers...))
	}

	// 2. 如果有预设密码，添加密码认证
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	} else {
		// 3. 创建交互式密码认证
		authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
			password, err := PromptPassword(fmt.Sprintf("%s's password: ", cfg.Username))
			cfg.Password = password
			return password, err
		}))
	}

	return &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境中应使用更安全的验证方式
	}, nil
}

// getAvailableSigners 获取可用的私钥签名器
func getAvailableSigners(privateKeyPath string) []ssh.Signer {
	var signers []ssh.Signer

	// 如果指定了私钥路径，优先使用
	if privateKeyPath != "" {
		if signer, err := loadPrivateKey(privateKeyPath); err == nil {
			signers = append(signers, signer)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load specified private key %s: %v\n", privateKeyPath, err)
		}
		return signers
	}

	// 尝试常见的默认私钥位置
	defaultKeys := []string{
		filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa"),
		filepath.Join(os.Getenv("HOME"), ".ssh", "id_ed25519"),
		filepath.Join(os.Getenv("HOME"), ".ssh", "id_ecdsa"),
		filepath.Join(os.Getenv("HOME"), ".ssh", "id_dsa"),
	}

	for _, keyPath := range defaultKeys {
		if fileExists(keyPath) {
			if signer, err := loadPrivateKey(keyPath); err == nil {
				signers = append(signers, signer)
			}
		}
	}

	return signers
}

// loadPrivateKey 加载私钥
func loadPrivateKey(privateKeyPath string) (ssh.Signer, error) {
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	return signer, nil
}

// PromptPassword 安全地提示用户输入密码
func PromptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println() // 换行
	return string(passwordBytes), nil
}

// fileExists 检查文件是否存在
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}
