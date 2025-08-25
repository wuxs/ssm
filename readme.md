# SSM - Simple SSH Manager

SSM (Simple SSH Manager) 是一个命令行工具，用于简化SSH连接管理。它提供了历史记录、密码记忆和窗口自适应等功能，使SSH连接更加便捷。

## 功能特性

- 🚀 **快速连接**: 通过历史记录快速连接到之前访问过的服务器
- 🔐 **多种认证方式**: 支持SSH密钥和密码认证
- 📚 **连接历史**: 自动保存连接历史，方便重复访问
- 🖥️ **窗口自适应**: 自动适应终端窗口大小变化
- 🔧 **兼容SSH语法**: 支持标准SSH客户端语法格式

## 安装

### 使用Go安装

```bash
go install github.com/wuxs/ssm@latest
```

### 从源码构建

```bash
git clone https://github.com/wuxs/ssm.git
cd ssm
go build -o ssm
```

## 使用方法

### 基本连接

```bash
# 使用默认SSH密钥连接
ssm user@hostname

# 指定端口
ssm user@hostname:2222

# 使用密码认证
ssm --password=yourpassword user@hostname

# 使用特定私钥
ssm -i ~/.ssh/specific_key user@hostname
```

### 查看历史记录

```bash
# 列出所有连接历史
ssm --list-history
```


### 命令行参数

```
Usage:
  ssm [user@]hostname[:port] [flags]

Flags:
  -h, --help              help for ssm
  -i, --identity string   Private key file for authentication (default is ~/.ssh/id_rsa)
  -l, --list-history      List SSH connection history
      --password string   Password for authentication
  -p, --port string       Port to connect to on the remote host
```

## 配置

SSM会自动在用户主目录下创建 `.ssm` 文件夹用于存储配置和历史记录：

- 历史记录文件: `~/.ssm/ssh_history.json`

## 安全注意事项

1. **密码存储**: 使用 `--password` 参数时，密码可能会以明文形式存储在历史记录中，请谨慎使用
2. **文件权限**: 历史记录文件设置为 0600 权限，仅允许所有者读写
3. **推荐做法**: 建议使用SSH密钥认证而非密码认证以提高安全性

## 贡献

欢迎提交Issue和Pull Request来改进SSM。

### 开发环境设置

```bash
git clone https://github.com/wuxs/ssm.git
cd ssm
go mod tidy
```

## 许可证

本项目采用 [MIT License](LICENSE) 开源许可证。

## 致谢

SSM使用了以下优秀的开源库：

- [Cobra](https://github.com/spf13/cobra) - Go命令行接口
- [x/crypto/ssh](https://golang.org/x/crypto/ssh) - Go SSH支持
- [x/term](https://golang.org/x/term) - Go终端控制

---

*注意: 这是一个个人项目，不隶属于任何公司或组织。使用时请自行承担风险。*