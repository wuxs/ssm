// cmd/root.go
package cmd

import (
	"fmt"
	"io"
	"net"
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
	Short: "Simple SSH Manager - Connect to remote servers and manage SSH connections",
	Long: `Simple SSH Manager (SSM) is a lightweight SSH connection management tool.
It simplifies SSH connections with intelligent configuration management,
multiple authentication methods, and jump host support.

Available Commands:
  cp         Copy files to/from remote servers using SFTP

Examples:
  ssm user@hostname                              # Connect to remote server
  ssm user@hostname:2222                         # Connect with custom port
  ssm -J jumphost user@target                    # Connect via jump host
  ssm -J user@jumphost:2222 user@target:22       # Connect via jump host with custom port
  ssm cp file.txt user@host:/remote/path         # Copy file to remote
  ssm cp user@host:/remote/file.txt ./           # Copy file from remote`,
	Args: cobra.MaximumNArgs(1),
	Run:  runSSHCommand,
}

func init() {
	rootCmd.Flags().StringP("identity", "i", "", "Private key file for authentication (default is ~/.ssh/id_rsa)")
	rootCmd.Flags().StringP("port", "p", "", "Port to connect to on the remote host")
	rootCmd.Flags().StringP("proxy-jump", "J", "", "Connect via jump host. Format: [user@]hostname[:port]")
	rootCmd.Flags().BoolP("list", "l", false, "List SSH connection configurations")
	rootCmd.Flags().StringP("delete", "d", "", "Delete SSH connection configuration by key (user@host:port)")
	rootCmd.Flags().StringSliceP("local-forward", "L", []string{}, "Local port forwarding, format: [local_port:]remote_host:remote_port")
	rootCmd.Flags().StringSliceP("remote-forward", "R", []string{}, "Remote port forwarding, format: [remote_port:]local_host:local_port")
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
	localForwards, _ := cmd.Flags().GetStringSlice("local-forward")
	remoteForwards, _ := cmd.Flags().GetStringSlice("remote-forward")
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
	if err := establishConnection(sshConfig, localForwards, remoteForwards); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}
}

func establishConnection(cfg *config.SSHConfig, localForwards, remoteForwards []string) error {
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

	// 如果有端口转发需求，则处理端口转发
	if len(localForwards) > 0 || len(remoteForwards) > 0 {
		// 处理本地端口转发 (-L)
		for _, forward := range localForwards {
			lf, err := parseLocalForward(forward)
			if err != nil {
				return fmt.Errorf("invalid local forward format '%s': %v", forward, err)
			}

			go func() {
				err := startLocalForward(client, lf)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Local forward failed for %s: %v\n", forward, err)
				}
			}()

			fmt.Printf("Local forwarding: %s:%d -> %s:%d\n", lf.bindAddr, lf.bindPort, lf.remoteHost, lf.remotePort)
		}

		// 处理远程端口转发 (-R)
		for _, forward := range remoteForwards {
			rf, err := parseRemoteForward(forward)
			if err != nil {
				return fmt.Errorf("invalid remote forward format '%s': %v", forward, err)
			}

			go func() {
				err := startRemoteForward(client, rf)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Remote forward failed for %s: %v\n", forward, err)
				}
			}()

			fmt.Printf("Remote forwarding: %s:%d <- %s:%d\n", rf.bindAddr, rf.bindPort, rf.localHost, rf.localPort)
		}
	}

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

// LocalForward 本地端口转发配置
type LocalForward struct {
	bindAddr   string // 本地绑定地址
	bindPort   uint16 // 本地绑定端口
	remoteHost string // 远程主机
	remotePort uint16 // 远程端口
}

// RemoteForward 远程端口转发配置
type RemoteForward struct {
	bindAddr  string // 远程绑定地址
	bindPort  uint16 // 远程绑定端口
	localHost string // 本地主机
	localPort uint16 // 本地端口
}

// parseLocalForward 解析本地端口转发参数
// 格式: [bind_addr:]bind_port:remote_host:remote_port 或 bind_port:remote_host:remote_port
func parseLocalForward(arg string) (*LocalForward, error) {
	parts := splitWithEscape(arg, ':')
	if len(parts) < 3 || len(parts) > 4 {
		return nil, fmt.Errorf("invalid format")
	}

	lf := &LocalForward{}

	if len(parts) == 4 {
		// 包含绑定地址
		lf.bindAddr = parts[0]
		lf.bindPort = parsePort(parts[1])
		lf.remoteHost = parts[2]
		lf.remotePort = parsePort(parts[3])
	} else {
		// 不包含绑定地址，默认为localhost
		lf.bindAddr = "localhost"
		lf.bindPort = parsePort(parts[0])
		lf.remoteHost = parts[1]
		lf.remotePort = parsePort(parts[2])
	}

	if lf.bindPort == 0 || lf.remotePort == 0 {
		return nil, fmt.Errorf("invalid port number")
	}

	return lf, nil
}

// parseRemoteForward 解析远程端口转发参数
// 格式: [bind_addr:]bind_port:local_host:local_port 或 bind_port:local_host:local_port
func parseRemoteForward(arg string) (*RemoteForward, error) {
	parts := splitWithEscape(arg, ':')
	if len(parts) < 3 || len(parts) > 4 {
		return nil, fmt.Errorf("invalid format")
	}

	rf := &RemoteForward{}

	if len(parts) == 4 {
		// 包含绑定地址
		rf.bindAddr = parts[0]
		rf.bindPort = parsePort(parts[1])
		rf.localHost = parts[2]
		rf.localPort = parsePort(parts[3])
	} else {
		// 不包含绑定地址，默认为localhost
		rf.bindAddr = "localhost"
		rf.bindPort = parsePort(parts[0])
		rf.localHost = parts[1]
		rf.localPort = parsePort(parts[2])
	}

	if rf.bindPort == 0 || rf.localPort == 0 {
		return nil, fmt.Errorf("invalid port number")
	}

	return rf, nil
}

// parsePort 将字符串转换为端口号
func parsePort(portStr string) uint16 {
	port := uint16(0)
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// splitWithEscape 拆分字符串，但忽略在方括号内的冒号
func splitWithEscape(s string, delimiter rune) []string {
	var parts []string
	var current string
	inBracket := false

	for _, r := range s {
		switch {
		case r == '[':
			inBracket = true
			current += string(r)
		case r == ']':
			inBracket = false
			current += string(r)
		case r == delimiter && !inBracket:
			parts = append(parts, current)
			current = ""
		default:
			current += string(r)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// startLocalForward 启动本地端口转发
func startLocalForward(client *ssh.Client, lf *LocalForward) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", lf.bindAddr, lf.bindPort))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %v", lf.bindAddr, lf.bindPort, err)
	}
	defer listener.Close()

	for {
		// 接受本地连接
		localConn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		// 连接到远程主机
		remoteAddr := fmt.Sprintf("%s:%d", lf.remoteHost, lf.remotePort)
		remoteConn, err := client.Dial("tcp4", remoteAddr)
		if err != nil {
			localConn.Close()
			fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", remoteAddr, err)
			continue
		}

		// 在两个连接之间转发数据
		go func() {
			defer localConn.Close()
			defer remoteConn.Close()
			copyConn(localConn, remoteConn)
		}()

		go func() {
			defer localConn.Close()
			defer remoteConn.Close()
			copyConn(remoteConn, localConn)
		}()
	}
}

// startRemoteForward 启动远程端口转发
func startRemoteForward(client *ssh.Client, rf *RemoteForward) error {
	// 监听远程端口
	remoteListener, err := client.Listen("tcp", fmt.Sprintf("%s:%d", rf.bindAddr, rf.bindPort))
	if err != nil {
		return fmt.Errorf("failed to listen on remote %s:%d: %v", rf.bindAddr, rf.bindPort, err)
	}
	defer remoteListener.Close()

	for {
		// 接受远程连接
		remoteConn, err := remoteListener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept remote connection: %v", err)
		}

		// 连接到本地主机
		localAddr := fmt.Sprintf("%s:%d", rf.localHost, rf.localPort)
		localConn, err := net.Dial("tcp4", localAddr)
		if err != nil {
			remoteConn.Close()
			fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", localAddr, err)
			continue
		}

		// 在两个连接之间转发数据
		go func() {
			defer localConn.Close()
			defer remoteConn.Close()
			copyConn(localConn, remoteConn)
		}()

		go func() {
			defer localConn.Close()
			defer remoteConn.Close()
			copyConn(remoteConn, localConn)
		}()
	}
}

// copyConn 在两个连接之间复制数据
func copyConn(dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
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
