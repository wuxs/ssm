// pkg/sftp/transfer.go
package sftp

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/wuxs/ssm/pkg/auth"
	"github.com/wuxs/ssm/pkg/config"
)

// TransferOptions 传输选项
type TransferOptions struct {
	Recursive bool // 递归传输目录
	Verbose   bool // 显示详细信息
	Preserve  bool // 保持文件属性
}

// TransferManager SFTP传输管理器
type TransferManager struct {
	sshClient *ssh.Client // 保存SSH客户端引用，用于执行命令
}

// NewTransferManager 创建新的传输管理器
func NewTransferManager() *TransferManager {
	return &TransferManager{}
}

// LocationInterface 位置接口
type LocationInterface interface {
	IsRemoteLocation() bool
	GetPath() string
	GetDisplayPath() string
}

// Transfer 执行文件传输
func (tm *TransferManager) Transfer(sshConfig *config.SSHConfig, source, destination LocationInterface, options *TransferOptions) error {
	if source.IsRemoteLocation() && destination.IsRemoteLocation() {
		return fmt.Errorf("remote to remote copy is not supported")
	}

	if !source.IsRemoteLocation() && !destination.IsRemoteLocation() {
		return fmt.Errorf("local to local copy should use regular cp command")
	}

	// 建立SSH连接
	client, err := tm.establishSSHConnection(sshConfig)
	if err != nil {
		return fmt.Errorf("failed to establish SSH connection: %v", err)
	}
	defer client.Close()
	tm.sshClient = client

	// 创建SFTP客户端
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %v", err)
	}
	defer sftpClient.Close()

	// 执行传输
	if source.IsRemoteLocation() {
		// 远程到本地
		return tm.downloadFile(sftpClient, source.GetPath(), destination.GetPath(), options)
	} else {
		// 本地到远程
		return tm.uploadFile(sftpClient, source.GetPath(), destination.GetPath(), options)
	}
}

// establishSSHConnection 建立SSH连接
func (tm *TransferManager) establishSSHConnection(cfg *config.SSHConfig) (*ssh.Client, error) {
	// 创建SSH客户端配置
	clientConfig, err := auth.CreateClientConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH config: %v", err)
	}

	// 连接SSH服务器（支持跳板机）
	return tm.connectWithJump(cfg, clientConfig)
}

// connectWithJump 支持跳板机的连接
func (tm *TransferManager) connectWithJump(cfg *config.SSHConfig, clientConfig *ssh.ClientConfig) (*ssh.Client, error) {
	// 如果没有跳板机，直接连接
	if cfg.ProxyJump == "" {
		addr := cfg.Host + ":" + cfg.Port
		if cfg.Host != "" && cfg.Port != "" {
			fmt.Printf("Connecting to %s...\n", addr)
		}
		return ssh.Dial("tcp", addr, clientConfig)
	}

	// 解析跳板机配置
	jumpConfig := tm.parseJumpConfig(cfg.ProxyJump)

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

// parseJumpConfig 解析跳板机配置
func (tm *TransferManager) parseJumpConfig(proxyJump string) *config.SSHConfig {
	// 这里需要导入utils包的解析函数
	username := ""
	hostname := ""
	port := "22"

	// 简单解析跳板机地址
	if strings.Contains(proxyJump, "@") {
		parts := strings.Split(proxyJump, "@")
		if len(parts) == 2 {
			username = parts[0]
			hostPort := parts[1]
			if strings.Contains(hostPort, ":") {
				hostPortParts := strings.Split(hostPort, ":")
				if len(hostPortParts) == 2 {
					hostname = hostPortParts[0]
					port = hostPortParts[1]
				}
			} else {
				hostname = hostPort
			}
		}
	} else {
		if strings.Contains(proxyJump, ":") {
			parts := strings.Split(proxyJump, ":")
			if len(parts) == 2 {
				hostname = parts[0]
				port = parts[1]
			}
		} else {
			hostname = proxyJump
		}
	}

	// 获取默认用户名
	if username == "" {
		if user := os.Getenv("USER"); user != "" {
			username = user
		} else {
			username = "root"
		}
	}

	key := fmt.Sprintf("%s@%s:%s", username, hostname, port)
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

// uploadFile 上传文件或目录
func (tm *TransferManager) uploadFile(sftpClient *sftp.Client, localPath, remotePath string, options *TransferOptions) error {
	localInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local path: %v", err)
	}
	if localInfo.IsDir() {
		if !options.Recursive {
			return fmt.Errorf("cannot copy directory without -r flag")
		}
		return tm.uploadDirectory(sftpClient, localPath, remotePath, options)
	}

	// 检查远程路径是否为目录，如果是则附加源文件名
	remotePath = tm.resolveRemotePath(sftpClient, localPath, remotePath)
	return tm.uploadSingleFile(sftpClient, localPath, remotePath, options)
}

// downloadFile 下载文件或目录
func (tm *TransferManager) downloadFile(sftpClient *sftp.Client, remotePath, localPath string, options *TransferOptions) error {
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote path: %v", err)
	}

	if remoteInfo.IsDir() {
		if !options.Recursive {
			return fmt.Errorf("cannot copy directory without -r flag")
		}
		return tm.downloadDirectory(sftpClient, remotePath, localPath, options)
	}

	// 检查本地路径是否为目录，如果是则附加源文件名
	localPath = tm.resolveLocalPath(remotePath, localPath)

	return tm.downloadSingleFile(sftpClient, remotePath, localPath, options)
}

// uploadSingleFile 上传单个文件
func (tm *TransferManager) uploadSingleFile(sftpClient *sftp.Client, localPath, remotePath string, options *TransferOptions) error {
	if options.Verbose {
		fmt.Printf("Uploading: %s -> %s\n", localPath, remotePath)
	}

	// 打开本地文件
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file '%s': %v", localPath, err)
	}
	defer localFile.Close()

	// 获取文件信息
	localInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get local file info: %v", err)
	}

	// 创建远程目录
	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		if err := sftpClient.MkdirAll(remoteDir); err != nil {
			return fmt.Errorf("failed to create remote directory '%s': %v", remoteDir, err)
		}
	}

	// 检查远程文件是否已存在
	if _, err := sftpClient.Stat(remotePath); err == nil {
		if options.Verbose {
			fmt.Printf("Warning: Remote file '%s' already exists, overwriting...\n", remotePath)
		}
	}

	// 创建远程文件
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file '%s': %v", remotePath, err)
	}
	defer remoteFile.Close()

	// 复制文件内容
	fileSize := localInfo.Size()
	written, err := tm.copyWithProgress(remoteFile, localFile, fileSize, options.Verbose)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %v", err)
	}

	if options.Verbose {
		fmt.Printf("✓ Uploaded %d bytes successfully\n", written)
	}

	// 保持文件属性
	if options.Preserve {
		if err := remoteFile.Chmod(localInfo.Mode()); err != nil {
			fmt.Printf("Warning: failed to set remote file mode: %v\n", err)
		}
		if err := sftpClient.Chtimes(remotePath, localInfo.ModTime(), localInfo.ModTime()); err != nil {
			fmt.Printf("Warning: failed to set remote file times: %v\n", err)
		}
	}

	return nil
}

// downloadSingleFile 下载单个文件
func (tm *TransferManager) downloadSingleFile(sftpClient *sftp.Client, remotePath, localPath string, options *TransferOptions) error {
	if options.Verbose {
		fmt.Printf("Downloading: %s -> %s\n", remotePath, localPath)
	}

	// 打开远程文件
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file '%s': %v", remotePath, err)
	}
	defer remoteFile.Close()

	// 获取远程文件信息
	remoteInfo, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get remote file info: %v", err)
	}

	// 创建本地目录
	localDir := filepath.Dir(localPath)
	if localDir != "." && localDir != "" {
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return fmt.Errorf("failed to create local directory '%s': %v", localDir, err)
		}
	}

	// 检查本地文件是否已存在
	if _, err := os.Stat(localPath); err == nil {
		if options.Verbose {
			fmt.Printf("Warning: Local file '%s' already exists, overwriting...\n", localPath)
		}
	}

	// 创建本地文件
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file '%s': %v", localPath, err)
	}
	defer localFile.Close()

	// 复制文件内容
	fileSize := remoteInfo.Size()
	written, err := tm.copyWithProgress(localFile, remoteFile, fileSize, options.Verbose)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %v", err)
	}

	if options.Verbose {
		fmt.Printf("✓ Downloaded %d bytes successfully\n", written)
	}

	// 保持文件属性
	if options.Preserve {
		if err := localFile.Chmod(remoteInfo.Mode()); err != nil {
			fmt.Printf("Warning: failed to set local file mode: %v\n", err)
		}
		if err := os.Chtimes(localPath, remoteInfo.ModTime(), remoteInfo.ModTime()); err != nil {
			fmt.Printf("Warning: failed to set local file times: %v\n", err)
		}
	}

	return nil
}

// uploadDirectory 上传目录
func (tm *TransferManager) uploadDirectory(sftpClient *sftp.Client, localDir, remoteDir string, options *TransferOptions) error {
	if options.Verbose {
		fmt.Printf("Uploading directory: %s -> %s\n", localDir, remoteDir)
	}

	// 创建远程目录
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("failed to create remote directory '%s': %v", remoteDir, err)
	}

	// 统计文件数量
	var totalFiles int
	var processedFiles int

	if options.Verbose {
		filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				totalFiles++
			}
			return nil
		})
		fmt.Printf("Found %d files to upload\n", totalFiles)
	}

	// 遍历本地目录
	return filepath.Walk(localDir, func(localPath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking directory '%s': %v", localPath, err)
		}

		// 计算相对路径
		relPath, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %v", err)
		}

		// 跳过根目录本身
		if relPath == "." {
			return nil
		}

		remotePath := filepath.Join(remoteDir, relPath)
		// 转换路径分隔符为Unix风格
		remotePath = strings.ReplaceAll(remotePath, "\\", "/")

		if info.IsDir() {
			if options.Verbose {
				fmt.Printf("Creating directory: %s\n", remotePath)
			}
			return sftpClient.MkdirAll(remotePath)
		} else {
			processedFiles++
			if options.Verbose {
				fmt.Printf("Progress: [%d/%d] ", processedFiles, totalFiles)
			}
			return tm.uploadSingleFile(sftpClient, localPath, remotePath, options)
		}
	})
}

// downloadDirectory 下载目录
func (tm *TransferManager) downloadDirectory(sftpClient *sftp.Client, remoteDir, localDir string, options *TransferOptions) error {
	if options.Verbose {
		fmt.Printf("Downloading directory: %s -> %s\n", remoteDir, localDir)
	}

	// 创建本地目录
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory '%s': %v", localDir, err)
	}

	// 统计文件数量
	var totalFiles int
	var processedFiles int

	if options.Verbose {
		tm.walkRemoteDir(sftpClient, remoteDir, func(path string, info os.FileInfo) error {
			if !info.IsDir() {
				totalFiles++
			}
			return nil
		})
		fmt.Printf("Found %d files to download\n", totalFiles)
	}

	// 递归遍历远程目录
	return tm.walkRemoteDir(sftpClient, remoteDir, func(remotePath string, info os.FileInfo) error {
		// 计算相对路径
		relPath := strings.TrimPrefix(remotePath, remoteDir)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			return nil // 跳过根目录
		}

		localPath := filepath.Join(localDir, relPath)

		if info.IsDir() {
			if options.Verbose {
				fmt.Printf("Creating directory: %s\n", localPath)
			}
			return os.MkdirAll(localPath, 0755)
		} else {
			processedFiles++
			if options.Verbose {
				fmt.Printf("Progress: [%d/%d] ", processedFiles, totalFiles)
			}
			return tm.downloadSingleFile(sftpClient, remotePath, localPath, options)
		}
	})
}

// walkRemoteDir 递归遍历远程目录
func (tm *TransferManager) walkRemoteDir(sftpClient *sftp.Client, dir string, walkFn func(path string, info os.FileInfo) error) error {
	files, err := sftpClient.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read remote directory '%s': %v", dir, err)
	}

	for _, file := range files {
		fullPath := strings.TrimRight(dir, "/") + "/" + file.Name()

		if err := walkFn(fullPath, file); err != nil {
			return err
		}

		if file.IsDir() {
			if err := tm.walkRemoteDir(sftpClient, fullPath, walkFn); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyWithProgress 带进度显示的文件复制
func (tm *TransferManager) copyWithProgress(dst io.Writer, src io.Reader, totalSize int64, verbose bool) (int64, error) {
	if !verbose || totalSize <= 0 {
		return io.Copy(dst, src)
	}

	// 使用更大的缓冲区提高传输效率
	buffer := make([]byte, 64*1024) // 64KB buffer
	var written int64
	var lastPercent int = -1

	for {
		nr, er := src.Read(buffer)
		if nr > 0 {
			nw, ew := dst.Write(buffer[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}

			// 显示进度 - 更频繁的更新
			percent := int(written * 100 / totalSize)
			if percent != lastPercent && (percent%5 == 0 || percent == 100) {
				fmt.Printf("  %d%% (%s/%s)\n",
					percent,
					tm.formatBytes(written),
					tm.formatBytes(totalSize))
				lastPercent = percent
			}
		}
		if er != nil {
			if er != io.EOF {
				return written, er
			}
			break
		}
	}

	return written, nil
}

// formatBytes 格式化字节数为可读的字符串
func (tm *TransferManager) formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// resolveRemotePath 解析远程路径，如果是目录则附加源文件名
func (tm *TransferManager) resolveRemotePath(sftpClient *sftp.Client, localPath, remotePath string) string {
	// 解析远程路径中的 ~
	remotePath = tm.expandRemoteTilde(sftpClient, remotePath)
	// 如果远程路径以 / 结尾，说明用户明确指定了目录
	if strings.HasSuffix(remotePath, "/") {
		baseFileName := filepath.Base(localPath)
		return strings.TrimRight(remotePath, "/") + "/" + baseFileName
	}

	// 尝试检查远程路径是否存在且为目录
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err == nil && remoteInfo.IsDir() {
		// 远程路径是一个目录，附加源文件名
		baseFileName := filepath.Base(localPath)
		return strings.TrimRight(remotePath, "/") + "/" + baseFileName
	}
	// 远程路径不存在或是文件，直接使用
	return remotePath
}

// expandRemoteTilde 将远程路径中的 ~ 扩展为用户的 HOME 目录
func (tm *TransferManager) expandRemoteTilde(sftpClient *sftp.Client, path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	homeDir := tm.getRemoteHomeDir(sftpClient)
	// 如果只是 ~，替换为 /home/username 或 /root
	if path == "~" {
		// 获取远程用户的 HOME 目录
		if homeDir != "" {
			return homeDir
		}
		return path // 如果无法获取，保持原样
	}

	// 如果是 ~/path 的形式
	if strings.HasPrefix(path, "~/") {
		if homeDir != "" {
			// 替换 ~ 为 HOME 目录
			return homeDir + path[1:]
		}
		return path // 如果无法获取，保持原样
	}

	// 其他形式的 ~，比如 ~user，暂不处理
	return path
}

// getRemoteHomeDir 获取远程用户的 HOME 目录
func (tm *TransferManager) getRemoteHomeDir(sftpClient *sftp.Client) string {
	// 确保我们有 SSH 客户端引用
	if tm.sshClient == nil {
		log.Println("SSH client is not initialized")
		return ""
	}

	// 创建一个新的会话来执行命令
	session, err := tm.sshClient.NewSession()
	if err != nil {
		log.Println("Failed to create new SSH session:", err)
		return ""
	}
	defer session.Close()

	// 执行命令获取 HOME 环境变量
	output, err := session.Output("echo $HOME")
	if err == nil && len(output) > 0 {
		homeDir := strings.TrimSpace(string(output))
		if homeDir != "" && homeDir != "$HOME" {
			return homeDir
		}
	}

	// 备用方法：尝试使用 whoami 获取用户名然后构造 HOME 路径
	output, err = session.Output("whoami")
	if err == nil && len(output) > 0 {
		username := strings.TrimSpace(string(output))
		if username != "" {
			if username == "root" {
				return "/root"
			}
			return "/home/" + username
		}
	}

	return ""
}

// resolveLocalPath 解析本地路径，如果是目录则附加源文件名
func (tm *TransferManager) resolveLocalPath(remotePath, localPath string) string {
	// 如果本地路径以 / 或 \ 结尾，说明用户明确指定了目录
	if strings.HasSuffix(localPath, "/") || strings.HasSuffix(localPath, "\\") {
		baseFileName := filepath.Base(remotePath)
		return filepath.Join(localPath, baseFileName)
	}

	// 尝试检查本地路径是否存在且为目录
	localInfo, err := os.Stat(localPath)
	if err == nil && localInfo.IsDir() {
		// 本地路径是一个目录，附加源文件名
		baseFileName := filepath.Base(remotePath)
		return filepath.Join(localPath, baseFileName)
	}

	// 本地路径不存在或是文件，直接使用
	return localPath
}
