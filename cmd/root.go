// cmd/root.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// SSHConnectionConfig 表示SSH连接配置项
type SSHConnectionConfig struct {
	Host       string `json:"host"`
	Username   string `json:"username"`
	Port       string `json:"port"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
	LastUsed   string `json:"last_used"`
}

// SSHConfigStore 表示SSH连接配置存储
type SSHConfigStore struct {
	Items map[string]SSHConnectionConfig `json:"items"`
}

// sshCmd represents the ssh command
var rootCmd = &cobra.Command{
	Use:   "ssh [user@]hostname[:port]",
	Short: "Connect to a remote server via SSH",
	Long: `Connect to a remote server using SSH protocol.
Example:
  ssm user@hostname
  ssm user@hostname:2222
  ssm hostname`,
	Run: runSSHCommand,
}

// runSSHCommand 是SSH命令的主要执行函数
func runSSHCommand(cmd *cobra.Command, args []string) {
	// 检查是否要列出连接配置
	listConfigs, _ := cmd.Flags().GetBool("list")
	if listConfigs {
		displaySSHConfigs()
		return
	}

	// 检查是否要删除连接配置
	deleteConfig, _ := cmd.Flags().GetString("delete")
	if deleteConfig != "" {
		handleDeleteConfig(deleteConfig)
		return
	}

	// 如果没有参数，显示帮助信息
	if len(args) < 1 {
		cmd.Help()
		return
	}

	host := args[0]
	
	// 解析主机信息
	username, hostname, port := parseSSHHost(host)
	
	// 设置默认值
	username = getDefaultUsername(username)
	port = getDefaultPort(port)

	// 获取标志
	password, _ := cmd.Flags().GetString("password")
	privateKeyPath, _ := cmd.Flags().GetString("identity")
	
	// 检查连接配置中是否存在
	key := fmt.Sprintf("%s@%s:%s", username, hostname, port)
	connectionConfig, exists := getConnectionConfig(key)
	if password != ""{
		connectionConfig.Password = password
	}
	if privateKeyPath != ""{
		connectionConfig.PrivateKey = privateKeyPath
	}
	
	if exists {
		// 使用连接配置中的信息连接
		connectUsingConfig(connectionConfig)
		saveConnectionConfig(connectionConfig)
		return
	}
	
	// 获取默认私钥路径
	privateKeyPath = getDefaultPrivateKeyPath(privateKeyPath)
	
	// 创建新的连接配置项
	newConnectionConfig := SSHConnectionConfig{
		Host:       hostname,
		Username:   username,
		Port:       port,
		PrivateKey: privateKeyPath,
		Password:   password,
		LastUsed:   time.Now().Format(time.RFC3339),
	}
	
	// 保存到连接配置
	if err := saveConnectionConfig(newConnectionConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save connection config: %v\n", err)
	}
	
	establishSSHConnection(newConnectionConfig)
}

func init() {
	// 添加SSH相关标志
	rootCmd.Flags().StringP("identity", "i", "", "Private key file for authentication (default is ~/.ssh/id_rsa)")
	rootCmd.Flags().StringP("port", "p", "", "Port to connect to on the remote host")
	rootCmd.Flags().String("password", "", "Password for authentication")
	rootCmd.Flags().BoolP("list", "l", false, "List SSH connection configurations")
	rootCmd.Flags().StringP("delete", "d", "", "Delete SSH connection configuration by key (user@host:port)")
}

// Execute 启动命令执行
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// parseSSHHost 解析主机信息
func parseSSHHost(host string) (username, hostname, port string) {
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

	return username, hostname, port
}

// getDefaultUsername 获取默认用户名
func getDefaultUsername(username string) string {
	if username != "" {
		return username
	}
	
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	
	return "root"
}

// getDefaultPort 获取默认端口
func getDefaultPort(port string) string {
	if port != "" {
		return port
	}
	return "22"
}

// getDefaultPrivateKeyPath 获取默认私钥路径
func getDefaultPrivateKeyPath(privateKeyPath string) string {
	if privateKeyPath != "" {
		return privateKeyPath
	}
	return filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
}

// establishSSHConnection 建立SSH连接
func establishSSHConnection(config SSHConnectionConfig) {
	// 配置SSH客户端
	clientConfig, err := createSSHClientConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create SSH config: %v\n", err)
		return
	}

	// 连接服务器
	addr := config.Host + ":" + config.Port
	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to dial: %v\n", err)
		return
	}
	defer client.Close()

	// 创建会话
	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create session: %v\n", err)
		return
	}
	defer session.Close()

	// 设置终端为原始模式
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to make terminal raw: %v\n", err)
		return
	}
	defer term.Restore(fd, state)

	// 设置会话的输入输出
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// 获取初始窗口大小
	w, h, err := getTerminalSize(fd)
	if err != nil {
		w, h = 80, 40 // 默认大小
	}

	// 请求PTY，使用实际窗口大小
	if err := session.RequestPty("xterm-256color", h, w, ssh.TerminalModes{}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to request pty: %v\n", err)
		return
	}

	// 启动shell
	if err := session.Shell(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start shell: %v\n", err)
		return
	}

	// 启动goroutine监听窗口大小变化
	go monitorWindowSize(fd, session)

	// 等待会话结束
	session.Wait()
}

// createSSHClientConfig 创建SSH客户端配置
func createSSHClientConfig(config SSHConnectionConfig) (*ssh.ClientConfig, error) {
	var authMethods []ssh.AuthMethod
	
	// 如果提供了私钥路径，则添加私钥认证
	if config.PrivateKey != "" {
		signer, err := loadPrivateKey(config.PrivateKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load private key: %v\n", err)
		} else {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	
	// 如果提供了密码，则添加密码认证
	if config.Password != "" {
		authMethods = append(authMethods, ssh.Password(config.Password))
	}
	
	clientConfig := &ssh.ClientConfig{
		User:            config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境中应使用更安全的验证方式
	}
	
	return clientConfig, nil
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

// getTerminalSize 获取终端窗口大小
func getTerminalSize(fd int) (int, int, error) {
	ws := &unix.Winsize{}
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return 0, 0, fmt.Errorf("failed to get terminal size")
	}
	return int(ws.Col), int(ws.Row), nil
}

// monitorWindowSize 监听窗口大小变化
func monitorWindowSize(fd int, session *ssh.Session) {
	// 创建信号通道监听窗口大小变化
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// 立即发送一次初始大小
	if w, h, err := getTerminalSize(fd); err == nil {
		session.WindowChange(h, w)
	}

	// 持续监听窗口大小变化
	for range sigChan {
		if w, h, err := getTerminalSize(fd); err == nil {
			session.WindowChange(h, w)
		}
	}
}

// getConfigFilePath 获取配置文件路径
func getConfigFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}
	return filepath.Join(homeDir, ".ssm", "ssh_config.json")
}

// loadSSHConfigs 加载SSH连接配置
func loadSSHConfigs() (*SSHConfigStore, error) {
	configFile := getConfigFilePath()

	// 如果文件不存在，返回空配置存储
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return &SSHConfigStore{Items: make(map[string]SSHConnectionConfig)}, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var configStore SSHConfigStore
	if err := json.Unmarshal(data, &configStore); err != nil {
		return nil, err
	}

	// 确保 Items 不为 nil
	if configStore.Items == nil {
		configStore.Items = make(map[string]SSHConnectionConfig)
	}

	return &configStore, nil
}

// saveSSHConfigs 保存SSH连接配置
func saveSSHConfigs(configStore *SSHConfigStore) error {
	configFile := getConfigFilePath()

	// 确保目录存在
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(configStore, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, data, 0600)
}

// saveConnectionConfig 保存连接配置项
func saveConnectionConfig(config SSHConnectionConfig) error {
	configStore, err := loadSSHConfigs()
	if err != nil {
		return err
	}

	// 使用 user@host:port 作为键
	key := fmt.Sprintf("%s@%s:%s", config.Username, config.Host, config.Port)
	configStore.Items[key] = config

	return saveSSHConfigs(configStore)
}

// getConnectionConfig 获取连接配置项
func getConnectionConfig(key string) (SSHConnectionConfig, bool) {
	configStore, err := loadSSHConfigs()
	if err != nil || configStore.Items == nil {
		return SSHConnectionConfig{}, false
	}
	
	config, exists := configStore.Items[key]
	return config, exists
}

// displaySSHConfigs 列出SSH连接配置
func displaySSHConfigs() {
	configStore, err := loadSSHConfigs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configs: %v\n", err)
		return
	}

	if len(configStore.Items) == 0 {
		fmt.Println("No SSH connection configurations found.")
		return
	}

	fmt.Println("SSH Connection Configurations:")

	// 按最后使用时间排序
	type configEntry struct {
		Key    string
		Config SSHConnectionConfig
	}

	entries := make([]configEntry, 0, len(configStore.Items))
	for k, v := range configStore.Items {
		entries = append(entries, configEntry{Key: k, Config: v})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Config.LastUsed > entries[j].Config.LastUsed
	})

	for i, entry := range entries {
		authMethod := getAuthMethodDescription(entry.Config)
		fmt.Printf("%d. %s %s (%s)\n", i+1, entry.Config.LastUsed, entry.Key, authMethod)
	}
}

// getAuthMethodDescription 获取认证方式描述
func getAuthMethodDescription(config SSHConnectionConfig) string {
	switch {
	case config.PrivateKey != "" && config.Password != "":
		return "password+key"
	case config.PrivateKey != "":
		return "key"
	case config.Password != "":
		return "password"
	default:
		return "unknown"
	}
}

// connectUsingConfig 使用连接配置进行连接
func connectUsingConfig(config SSHConnectionConfig) {
	// 更新最后使用时间
	config.LastUsed = time.Now().Format(time.RFC3339)
	if err := saveConnectionConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update config: %v\n", err)
	}

	// 使用连接配置中的信息连接
	establishSSHConnection(config)
}

// handleDeleteConfig 处理删除连接配置
func handleDeleteConfig(key string) {
	if err := deleteConnectionConfig(key); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to delete config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully deleted connection config: %s\n", key)
}

// deleteConnectionConfig 删除连接配置项
func deleteConnectionConfig(key string) error {
	configStore, err := loadSSHConfigs()
	if err != nil {
		return err
	}

	// 检查配置是否存在
	if _, exists := configStore.Items[key]; !exists {
		return fmt.Errorf("connection config not found: %s", key)
	}

	// 删除配置
	delete(configStore.Items, key)

	// 保存更新后的配置存储
	return saveSSHConfigs(configStore)
}