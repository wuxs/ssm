// cmd/ssh.go
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

// SSHHistoryItem 表示SSH连接历史记录项
type SSHHistoryItem struct {
	Host       string `json:"host"`
	Username   string `json:"username"`
	Port       string `json:"port"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
	LastUsed   string `json:"last_used"`
}

// SSHHistory 表示SSH连接历史记录
type SSHHistory struct {
	Items map[string]SSHHistoryItem `json:"items"`
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
	Run: func(cmd *cobra.Command, args []string) {
		// 检查是否要列出历史记录
		listHistory, _ := cmd.Flags().GetBool("list-history")
		if listHistory {
			listSSHHistory()
			return
		}

		// 如果没有参数，显示帮助信息
		if len(args) < 1 {
			cmd.Help()
			return
		}

		host := args[0]
		// 检查是否在历史记录中存在
		history, _ := loadSSHHistory()
		if history.Items == nil {
			history.Items = make(map[string]SSHHistoryItem)
		}

		// 解析主机信息
		username, hostname, port := parseSSHHost(host)

		if username == "" {
			username = os.Getenv("USER")
			if username == "" {
				username = "root"
			}
		}

		if port == "" {
			port = "22"
		}

		// 获取标志
		password, _ := cmd.Flags().GetString("password")
		privateKeyPath, _ := cmd.Flags().GetString("identity")
		// 检查历史记录中是否存在
		key := fmt.Sprintf("%s@%s:%s", username, hostname, port)
		if item, exists := history.Items[key]; exists {
			// 使用历史记录中的信息连接
			connectFromHistoryItem(item)
			return
		}
		if privateKeyPath == "" {
			privateKeyPath = filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")
		}

		// 创建历史记录项
		historyItem := SSHHistoryItem{
			Host:       hostname,
			Username:   username,
			Port:       port,
			PrivateKey: privateKeyPath,
			Password:   password,
			LastUsed:   time.Now().Format(time.RFC3339),
		}

		// 保存到历史记录
		if err := addToHistory(historyItem); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save to history: %v\n", err)
		}

		connectSSH(username, hostname, port, privateKeyPath, password)
	},
}

func init() {
	// 添加SSH相关标志
	rootCmd.Flags().StringP("identity", "i", "", "Private key file for authentication (default is ~/.ssh/id_rsa)")
	rootCmd.Flags().StringP("port", "p", "", "Port to connect to on the remote host")
	rootCmd.Flags().String("password", "", "Password for authentication")
	rootCmd.Flags().BoolP("list-history", "l", false, "List SSH connection history")

}
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// 解析主机信息
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

// 建立SSH连接
func connectSSH(username, hostname, port, privateKeyPath, password string) {
	// 读取私钥
	key, err := os.ReadFile(privateKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read private key: %v\n", err)
		return
	}

	// 解析私钥
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse private key: %v\n", err)
		return
	}

	// 配置SSH客户端
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境中应使用更安全的验证方式
	}

	// 连接服务器
	addr := hostname + ":" + port
	client, err := ssh.Dial("tcp", addr, config)
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
	go watchWindowSize(fd, session)

	// 等待会话结束
	session.Wait()
}

// 获取终端窗口大小
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

// 监听窗口大小变化
func watchWindowSize(fd int, session *ssh.Session) {
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

// 获取历史记录文件路径
func getHistoryFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}
	return filepath.Join(homeDir, ".ssm", "ssh_history.json")
}

// 加载SSH历史记录
func loadSSHHistory() (*SSHHistory, error) {
	historyFile := getHistoryFilePath()

	// 如果文件不存在，返回空历史记录
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		return &SSHHistory{Items: make(map[string]SSHHistoryItem)}, nil
	}

	data, err := os.ReadFile(historyFile)
	if err != nil {
		return nil, err
	}

	var history SSHHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	// 确保 Items 不为 nil
	if history.Items == nil {
		history.Items = make(map[string]SSHHistoryItem)
	}

	return &history, nil
}

// 保存SSH历史记录
func saveSSHHistory(history *SSHHistory) error {
	historyFile := getHistoryFilePath()

	// 确保目录存在
	dir := filepath.Dir(historyFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0600)
}

// 添加到历史记录
func addToHistory(item SSHHistoryItem) error {
	history, err := loadSSHHistory()
	if err != nil {
		return err
	}

	// 使用 user@host:port 作为键
	key := fmt.Sprintf("%s@%s:%s", item.Username, item.Host, item.Port)
	history.Items[key] = item

	return saveSSHHistory(history)
}

// 列出SSH历史记录
func listSSHHistory() {
	history, err := loadSSHHistory()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load history: %v\n", err)
		return
	}

	if len(history.Items) == 0 {
		fmt.Println("No SSH connection history found.")
		return
	}

	fmt.Println("SSH Connection History:")

	// 按最后使用时间排序
	type historyEntry struct {
		Key  string
		Item SSHHistoryItem
	}

	entries := make([]historyEntry, 0, len(history.Items))
	for k, v := range history.Items {
		entries = append(entries, historyEntry{Key: k, Item: v})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Item.LastUsed > entries[j].Item.LastUsed
	})

	for i, entry := range entries {
		authMethod := "key"
		if entry.Item.PrivateKey != "" && entry.Item.Password != "" {
			authMethod = "password+key"
		} else if entry.Item.PrivateKey != "" {
			authMethod = "key"
		} else if entry.Item.Password != "" {
			authMethod = "password"
		}
		fmt.Printf("%d. %s %s (%s)\n", i+1, entry.Item.LastUsed, entry.Key, authMethod)
	}
}

// 从历史记录项连接
func connectFromHistoryItem(item SSHHistoryItem) {
	// 更新最后使用时间
	item.LastUsed = time.Now().Format(time.RFC3339)
	if err := addToHistory(item); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update history: %v\n", err)
	}

	// 使用历史记录中的信息连接
	connectSSH(item.Username, item.Host, item.Port, item.PrivateKey, item.Password)
}
