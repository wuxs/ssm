// pkg/terminal/terminal.go
package terminal

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// TerminalManager 终端管理器
type TerminalManager struct{}

// NewTerminalManager 创建终端管理器
func NewTerminalManager() *TerminalManager {
	return &TerminalManager{}
}

// SSHSession SSH会话接口
type SSHSession interface {
	RequestPty(term string, h, w int, termmodes ssh.TerminalModes) error
	Shell() error
	Wait() error
	WindowChange(h, w int) error
	Close() error
}

// SessionConfig 会话配置
type SessionConfig struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

// StartInteractiveSession 启动交互式SSH会话
func (tm *TerminalManager) StartInteractiveSession(session SSHSession, config *SessionConfig) error {
	// 设置会话的输入输出
	if config == nil {
		config = &SessionConfig{
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
	}

	// 设置终端为原始模式
	fd := int(config.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to make terminal raw: %v", err)
	}
	defer term.Restore(fd, state)

	// 获取初始窗口大小
	w, h, err := tm.getTerminalSize(fd)
	if err != nil {
		w, h = 80, 40 // 默认大小
	}

	// 请求PTY，使用实际窗口大小
	if err := session.RequestPty("xterm-256color", h, w, ssh.TerminalModes{}); err != nil {
		return fmt.Errorf("failed to request pty: %v", err)
	}

	// 启动shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %v", err)
	}

	// 启动goroutine监听窗口大小变化
	go tm.monitorWindowSize(fd, session)

	// 等待会话结束
	return session.Wait()
}

// getTerminalSize 获取终端窗口大小
func (tm *TerminalManager) getTerminalSize(fd int) (int, int, error) {
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
func (tm *TerminalManager) monitorWindowSize(fd int, session SSHSession) {
	// 创建信号通道监听窗口大小变化
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	// 立即发送一次初始大小
	if w, h, err := tm.getTerminalSize(fd); err == nil {
		session.WindowChange(h, w)
	}

	// 持续监听窗口大小变化
	for range sigChan {
		if w, h, err := tm.getTerminalSize(fd); err == nil {
			session.WindowChange(h, w)
		}
	}
}
