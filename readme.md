# 🚀 SSM (Simple SSH Manager)

[![Go Version](https://img.shields.io/badge/Go-1.23.8-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey.svg)](#)

SSM 是一个轻量级的 SSH 连接管理工具，旨在简化和优化 SSH 连接体验。通过智能的历史记录管理、多种认证方式支持和跳板机功能，让 SSH 连接变得更加便捷高效。

## ✨ 功能特性

### 🎯 核心功能
- **📚 历史记录管理**：自动保存连接历史，支持快速重连
- **🔄 智能配置复用**：自动复用认证信息，减少重复输入
- **📱 终端自适应**：自动适应终端窗口大小变化
- **⚡ 快速连接**：通过简洁的命令行接口实现一键连接
- **🔗 端口转发**：支持本地和远程端口转发

### 🔐 认证方式
- **🔑 SSH密钥认证**：支持多种密钥格式（RSA、Ed25519、ECDSA、DSA）
- **🔒 密码认证**：安全的交互式密码输入
- **🔀 混合认证**：智能选择最佳认证方式
- **📋 认证优先级**：publickey → password 标准SSH认证流程

### 🛡️ 安全特性
- **🔐 安全存储**：配置文件权限设置为 0600
- **🚫 无明文参数**：密码通过安全的交互式输入获取
- **✅ 连接验证**：只有连接成功后才保存配置
- **🔄 智能重试**：失败时不保存错误配置

### 🌐 兼容性
- **📡 跳板机支持**：完整的 ProxyJump 功能实现
- **🔌 端口转发**：支持本地(-L)和远程(-R)端口转发
- **🔧 标准SSH语法**：兼容标准SSH客户端语法
- **🖥️ 跨平台**：支持 Linux、macOS、Windows
- **🎨 终端兼容**：支持各种终端模拟器

## 📦 安装

### 方式一：从源码构建
```bash
git clone https://github.com/wuxs/ssm.git
cd ssm
go mod tidy
go build -o ssm
```

### 方式二：Go安装
```bash
go install github.com/wuxs/ssm@latest
```

## 🚀 快速开始

### 基本连接
```bash
# 连接到远程服务器
ssm user@hostname
ssm user@hostname:2222
ssm hostname  # 使用当前用户名
```

### 使用跳板机
```bash
# 通过跳板机连接
ssm -J jumphost user@target
ssm -J user@jumphost:2222 user@target:22
ssm --proxy-jump jump.example.com user@target.example.com
```

### 管理配置
```bash
# 列出所有保存的配置
ssm --list

# 删除指定配置
ssm --delete user@hostname:22
```

## 📋 命令行参数

### 🔗 连接参数
| 参数 | 短参数 | 说明 | 示例 |
|------|--------|------|------|
| `--identity` | `-i` | 指定私钥文件 | `-i ~/.ssh/id_rsa` |
| `--port` | `-p` | 指定端口 | `-p 2222` |

### 🌉 跳板机参数
| 参数 | 短参数 | 说明 | 示例 |
|------|--------|------|------|
| `--proxy-jump` | `-J` | 跳板机地址 | `-J user@jumphost:22` |

### 🛠️ 管理参数
| 参数 | 短参数 | 说明 | 示例 |
|------|--------|------|------|
| `--list` | `-l` | 列出所有配置 | `--list` |
| `--delete` | `-d` | 删除指定配置 | `--delete user@host:22` |
| `--help` | `-h` | 显示帮助信息 | `--help` |

## 🗂️ 配置管理

### 📁 配置文件位置
```
~/.ssm/ssh_config.json
```

### 🏗️ 配置文件结构
```json
{
  "items": {
    "user@hostname:22": {
      "host": "hostname",
      "username": "user",
      "port": "22",
      "private_key": "/home/user/.ssh/id_rsa",
      "password": "",
      "proxy_jump": "",
      "last_used": "2025-01-01T12:00:00Z"
    }
  }
}
```

### 🔄 自动管理功能
- ✅ **自动保存**：成功连接后自动保存配置
- ✅ **智能更新**：自动更新最后使用时间
- ✅ **配置复用**：自动复用已保存的认证信息
- ✅ **失败保护**：连接失败时不保存错误配置

## 🛡️ 安全注意事项

### 📋 推荐做法
- 🔑 **优先使用SSH密钥**：比密码认证更安全
- 🔐 **定期轮换密钥**：提高安全性
- 📂 **正确设置权限**：确保配置文件权限为 0600
- 🚫 **避免共享配置**：不要在多用户环境中共享配置文件

### 🛡️ 安全特性
- 🔒 **配置文件加密存储**：权限控制为 0600
- 🚫 **无命令行密码参数**：避免密码在进程列表中暴露
- ✅ **标准SSH认证流程**：遵循SSH客户端最佳实践
- 🔄 **智能认证重试**：减少认证失败风险

### ⚠️ 风险提示
- 密码会以明文形式存储在配置文件中，请谨慎使用
- 建议在生产环境中使用SSH密钥认证
- 定期检查和清理不需要的配置

## 🔧 故障排除

### ❓ 常见问题

**Q: 连接失败，提示 "Permission denied"**
A: 检查以下几个方面：
1. 确认用户名和主机地址是否正确
2. 检查SSH密钥权限：`chmod 600 ~/.ssh/id_rsa`
3. 确认目标服务器的SSH服务是否正常运行
4. 尝试使用标准ssh命令测试连接

**Q: 跳板机连接失败**
A: 排查步骤：
1. 先测试能否直接连接到跳板机
2. 确认跳板机支持端口转发功能
3. 检查跳板机的防火墙设置
4. 验证目标服务器从跳板机可达

**Q: 端口转发不工作**
A: 检查项目：
1. 确认端口转发参数格式正确
2. 检查本地/远程端口是否已被占用
3. 确认防火墙允许相关端口通信
4. 查看是否有错误信息输出

**Q: 配置文件损坏或丢失**
A: 解决方法：
1. 检查 `~/.ssm/ssh_config.json` 文件是否存在
2. 验证JSON格式是否正确
3. 备份并重新创建配置文件
4. 重新连接以生成新的配置

**Q: 终端窗口大小不自适应**
A: 检查项目：
1. 确认终端支持 SIGWINCH 信号
2. 检查终端模拟器设置
3. 尝试重新调整终端窗口大小
4. 重启SSH会话

## 🤝 贡献指南

我们欢迎各种形式的贡献！

### 🐛 报告问题
- 在 GitHub Issues 中详细描述问题
- 提供复现步骤和环境信息
- 附上相关的错误日志

### 💡 提交功能请求
- 描述功能的用途和价值
- 提供具体的使用场景
- 考虑向后兼容性

### 🔧 代码贡献
1. Fork 项目仓库
2. 创建功能分支：`git checkout -b feature/new-feature`
3. 提交代码：`git commit -am 'Add new feature'`
4. 推送分支：`git push origin feature/new-feature`
5. 创建 Pull Request

### 📝 代码规范
- 遵循 Go 语言标准规范
- 添加必要的注释和文档
- 确保代码测试通过
- 保持向后兼容性

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🙏 致谢

感谢所有为这个项目做出贡献的开发者和用户！

---

**💡 提示**：如果您在使用过程中遇到任何问题，请随时在 [GitHub Issues](https://github.com/wuxs/ssm/issues) 中反馈。