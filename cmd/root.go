// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	"github.com/wuxs/ssm/pkg/auth"
	"github.com/wuxs/ssm/pkg/config"
	"github.com/wuxs/ssm/pkg/terminal"
	"github.com/wuxs/ssm/pkg/utils"
)

var rootCmd = &cobra.Command{
	Use:   "ssm [user@]hostname[:port]",
	Short: "Connect to a remote server via SSH",
	Long: `Connect to a remote server using SSH protocol with optional jump host support.

Examples:
  ssm user@hostname
  ssm user@hostname:2222
  ssm hostname
  ssm -J jumphost user@target                    # Connect via jump host
  ssm -J user@jumphost:2222 user@target:22       # Connect via jump host with custom port
  ssm --proxy-jump jump.example.com user@target.example.com`,
	Run: runSSHCommand,
}

func init() {
	rootCmd.Flags().StringP("identity", "i", "", "Private key file for authentication (default is ~/.ssh/id_rsa)")
	rootCmd.Flags().StringP("port", "p", "", "Port to connect to on the remote host")
	rootCmd.Flags().StringP("proxy-jump", "J", "", "Connect via jump host. Format: [user@]hostname[:port]")
	rootCmd.Flags().BoolP("list", "l", false, "List SSH connection configurations")
	rootCmd.Flags().StringP("delete", "d", "", "Delete SSH connection configuration by key (user@host:port)")
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func runSSHCommand(cmd *cobra.Command, args []string) {
	// 检查是否要列出连接配置
	if listConfigs, _ := cmd.Flags().GetBool("list"); listConfigs {
		displayConfigs()
		return
	}

	// 检查是否要删除连接配置
	if deleteKey, _ := cmd.Flags().GetString("delete"); deleteKey != "" {
		handleDelete(deleteKey)
		return
	}

	// 如果没有参数，显示帮助信息
	if len(args) < 1 {
		cmd.Help()
		return
	}

	host := args[0]

	// 解析主机信息
	username, hostname, port := utils.ParseSSHHost(host)

	// 获取标志
	privateKeyPath, _ := cmd.Flags().GetString("identity")
	proxyJump, _ := cmd.Flags().GetString("proxy-jump")
	privateKeyPath = utils.GetDefaultPrivateKeyPath(privateKeyPath)

	// 检查现有配置
	key := utils.GetConfigKey(username, hostname, port)
	sshConfig, exists := config.Get(key)
	if !exists {
		// 创建新配置
		sshConfig = &config.SSHConfig{
			Host:       hostname,
			Username:   username,
			Port:       port,
			PrivateKey: privateKeyPath,
			ProxyJump:  proxyJump,
		}
	} else {
		// 更新现有配置
		if privateKeyPath != "" {
			sshConfig.PrivateKey = privateKeyPath
		}
		if proxyJump != "" {
			sshConfig.ProxyJump = proxyJump
		}
	}

	// 建立SSH连接
	if err := establishConnection(sshConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
}

func establishConnection(cfg *config.SSHConfig) error {
	// 创建SSH客户端配置
	clientConfig, err := auth.CreateClientConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %v", err)
	}

	// 连接SSH服务器（支持跳板机）
	client, err := connectWithJump(cfg, clientConfig)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer client.Close()

	// 创建会话
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// 连接成功，更新并保存配置
	cfg.UpdateLastUsed()
	if err := config.SaveConfig(cfg); err != nil {
		fmt.Printf("Warning: Failed to save config: %v\n", err)
	}

	// 设置会话的输入输出
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// 启动交互式终端会话
	terminalManager := terminal.NewTerminalManager()
	return terminalManager.StartInteractiveSession(session, nil)
}

func connectWithJump(cfg *config.SSHConfig, clientConfig *ssh.ClientConfig) (*ssh.Client, error) {
	// 如果没有跳板机，直接连接
	if cfg.ProxyJump == "" {
		addr := cfg.Host + ":" + cfg.Port
		fmt.Printf("Connecting to %s...\n", addr)
		return ssh.Dial("tcp", addr, clientConfig)
	}

	// 解析跳板机配置
	jumpConfig := parseJumpConfig(cfg.ProxyJump)

	// 创建跳板机SSH配置
	jumpClientConfig, err := auth.CreateClientConfig(jumpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create jump host SSH config: %v", err)
	}

	// 连接跳板机
	jumpAddr := jumpConfig.Host + ":" + jumpConfig.Port
	fmt.Printf("Connecting to jump host %s...\n", jumpAddr)
	jumpClient, err := ssh.Dial("tcp", jumpAddr, jumpClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to jump host: %v", err)
	}

	// 保存跳板机配置（如果连接成功）
	jumpConfig.UpdateLastUsed()
	if err := config.SaveConfig(jumpConfig); err != nil {
		fmt.Printf("Warning: Failed to save jump host config: %v\n", err)
	}

	// 通过跳板机连接到目标服务器
	targetAddr := cfg.Host + ":" + cfg.Port
	fmt.Printf("Connecting to target host %s through jump host...\n", targetAddr)
	targetConn, err := jumpClient.Dial("tcp", targetAddr)
	if err != nil {
		jumpClient.Close()
		return nil, fmt.Errorf("failed to dial target through jump host: %v", err)
	}

	// 建立SSH连接
	ncc, chans, reqs, err := ssh.NewClientConn(targetConn, targetAddr, clientConfig)
	if err != nil {
		targetConn.Close()
		jumpClient.Close()
		return nil, fmt.Errorf("failed to establish SSH connection: %v", err)
	}

	return ssh.NewClient(ncc, chans, reqs), nil
}

func parseJumpConfig(proxyJump string) *config.SSHConfig {
	username, hostname, port := utils.ParseSSHHost(proxyJump)
	key := utils.GetConfigKey(username, hostname, port)
	// 先尝试从保存的配置中查找
	if jumpConfig, exists := config.Get(key); exists {
		return jumpConfig
	}

	return &config.SSHConfig{
		Host:     hostname,
		Username: username,
		Port:     port,
	}
}

func displayConfigs() {
	configs, err := config.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configs: %v\n", err)
		return
	}

	if len(configs) == 0 {
		fmt.Println("No SSH connection configurations found.")
		return
	}

	fmt.Println("SSH Connection Configurations:")
	for i, cfg := range configs {
		line := fmt.Sprintf("%d. %s", i+1, cfg.GetKey())
		if cfg.ProxyJump != "" {
			line += fmt.Sprintf(" via %s", cfg.ProxyJump)
		}
		fmt.Println(line)
	}
}

func handleDelete(key string) {
	if err := config.Delete(key); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to delete config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully deleted connection config: %s\n", key)
}
