// cmd/cp.go
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wuxs/ssm/pkg/config"
	"github.com/wuxs/ssm/pkg/sftp"
	"github.com/wuxs/ssm/pkg/utils"
)

var cpCmd = &cobra.Command{
	Use:   "cp [flags] source destination",
	Short: "Copy files to/from remote servers using SFTP",
	Long: `Copy files or directories between local and remote systems using SFTP protocol.
Supports jump hosts and recursive directory copying.

The source and destination can be:
- Local file/directory: /path/to/file
- Remote file/directory: [user@]hostname[:port]:/path/to/file

Examples:
  ssm cp file.txt user@host:/remote/path/          # Copy local file to remote
  ssm cp user@host:/remote/file.txt ./             # Copy remote file to local
  ssm cp -r local-dir user@host:/remote/           # Copy directory recursively
  ssm cp -J jumphost file.txt user@target:/path/  # Copy through jump host
  ssm cp -J jump.example.com user@host:/file ./   # Download through jump host`,
	Args: cobra.ExactArgs(2),
	Run:  runCopyCommand,
}

func init() {
	cpCmd.Flags().StringP("identity", "i", "", "Private key file for authentication")
	cpCmd.Flags().StringP("port", "p", "", "Port to connect to on the remote host")
	cpCmd.Flags().StringP("proxy-jump", "J", "", "Connect via jump host. Format: [user@]hostname[:port]")
	cpCmd.Flags().BoolP("recursive", "r", false, "Copy directories recursively")
	cpCmd.Flags().BoolP("verbose", "v", false, "Show verbose output")
	cpCmd.Flags().Bool("preserve", false, "Preserve file modes and timestamps")

	rootCmd.AddCommand(cpCmd)
}

func runCopyCommand(cmd *cobra.Command, args []string) {
	source := args[0]
	destination := args[1]

	// 获取标志
	privateKeyPath, _ := cmd.Flags().GetString("identity")
	portFlag, _ := cmd.Flags().GetString("port")
	proxyJump, _ := cmd.Flags().GetString("proxy-jump")
	recursive, _ := cmd.Flags().GetBool("recursive")
	verbose, _ := cmd.Flags().GetBool("verbose")
	preserve, _ := cmd.Flags().GetBool("preserve")

	privateKeyPath = utils.GetDefaultPrivateKeyPath(privateKeyPath)

	// 解析源和目标
	srcLocation, err := parseLocation(source, portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing source: %v\n", err)
		os.Exit(1)
	}

	dstLocation, err := parseLocation(destination, portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing destination: %v\n", err)
		os.Exit(1)
	}

	// 验证传输类型
	if srcLocation.IsRemoteLocation() == dstLocation.IsRemoteLocation() {
		fmt.Fprintf(os.Stderr, "Error: One of source or destination must be remote\n")
		os.Exit(1)
	}

	// 确定远程位置的配置
	var remoteLocation *RemoteLocationInfo
	if srcLocation.IsRemoteLocation() {
		if rl, ok := srcLocation.(*RemoteLocationInfo); ok {
			remoteLocation = rl
		}
	} else {
		if rl, ok := dstLocation.(*RemoteLocationInfo); ok {
			remoteLocation = rl
		}
	}

	if remoteLocation == nil {
		fmt.Fprintf(os.Stderr, "Error: Unable to determine remote location\n")
		os.Exit(1)
	}

	// 创建SSH配置
	sshConfig := createSSHConfigForLocation(remoteLocation, privateKeyPath, proxyJump)

	// 创建传输选项
	options := &sftp.TransferOptions{
		Recursive: recursive,
		Verbose:   verbose,
		Preserve:  preserve,
	}

	// 执行文件传输
	transferManager := sftp.NewTransferManager()
	if err := transferManager.Transfer(sshConfig, srcLocation, dstLocation, options); err != nil {
		fmt.Fprintf(os.Stderr, "Transfer failed: %v\n", err)
		os.Exit(1)
	}
}

// LocalLocationInfo 本地文件位置
type LocalLocationInfo struct {
	Path string
}

func (l *LocalLocationInfo) IsRemoteLocation() bool { return false }
func (l *LocalLocationInfo) GetPath() string        { return l.Path }
func (l *LocalLocationInfo) GetDisplayPath() string { return l.Path }

// RemoteLocationInfo 远程文件位置
type RemoteLocationInfo struct {
	Username string
	Hostname string
	Port     string
	Path     string
}

func (r *RemoteLocationInfo) IsRemoteLocation() bool { return true }
func (r *RemoteLocationInfo) GetPath() string        { return r.Path }
func (r *RemoteLocationInfo) GetDisplayPath() string {
	return fmt.Sprintf("%s@%s:%s", r.Username, r.Hostname, r.Path)
}

// LocationInterface 位置接口
type LocationInterface interface {
	IsRemoteLocation() bool
	GetPath() string
	GetDisplayPath() string
}

// parseLocation 解析位置字符串
func parseLocation(location, defaultPort string) (LocationInterface, error) {
	// 检查是否是远程位置 (包含 :)
	if !strings.Contains(location, ":") {
		// 本地位置
		return &LocalLocationInfo{
			Path: location,
		}, nil
	}

	// 查找路径分隔符的位置
	pathSepIndex := strings.LastIndex(location, ":")
	if pathSepIndex == -1 {
		return nil, fmt.Errorf("invalid location format: %s", location)
	}

	// 分离主机部分和路径部分
	hostPart := location[:pathSepIndex]
	pathPart := location[pathSepIndex+1:]

	// 如果路径分隔符前面的部分看起来像是驱动器字母 (Windows)，则认为是本地路径
	if len(hostPart) == 1 && ((hostPart[0] >= 'A' && hostPart[0] <= 'Z') || (hostPart[0] >= 'a' && hostPart[0] <= 'z')) {
		return &LocalLocationInfo{
			Path: location,
		}, nil
	}

	// 解析远程主机信息
	username, hostname, port := utils.ParseSSHHost(hostPart)
	if defaultPort != "" {
		port = defaultPort
	}

	return &RemoteLocationInfo{
		Username: username,
		Hostname: hostname,
		Port:     port,
		Path:     pathPart,
	}, nil
}

// createSSHConfigForLocation 为位置创建SSH配置
func createSSHConfigForLocation(location *RemoteLocationInfo, privateKeyPath, proxyJump string) *config.SSHConfig {
	// 尝试从现有配置中获取
	key := utils.GetConfigKey(location.Username, location.Hostname, location.Port)
	if sshConfig, exists := config.Get(key); exists {
		// 更新配置
		if privateKeyPath != "" {
			sshConfig.PrivateKey = privateKeyPath
		}
		if proxyJump != "" {
			sshConfig.ProxyJump = proxyJump
		}
		return sshConfig
	}

	// 创建新配置
	return &config.SSHConfig{
		Host:       location.Hostname,
		Username:   location.Username,
		Port:       location.Port,
		PrivateKey: privateKeyPath,
		ProxyJump:  proxyJump,
	}
}
