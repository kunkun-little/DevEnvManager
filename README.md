# 开发环境管家 0.3.0

中文桌面工具，面向 Windows 10/11 x64。采用 Go + Wails v2 + Vue；界面和管理逻辑都在一个 EXE 内，不依赖同目录脚本、不使用 PowerShell 启动。需要 Microsoft Edge WebView2 Runtime；如机器没有，从[微软官方页面](https://developer.microsoft.com/microsoft-edge/webview2/)安装 x64 Evergreen Runtime。应用不修改执行策略，不静默安装运行时。

## 使用

1. 把 ZIP 完整解压到一个普通可写的本地目录，双击 `DevEnvManager.exe`。首次使用先看「空间总览」：C 盘剩余空间与识别出的开发缓存分开显示。
2. 点左侧环境，顶部显示当前版本与适用的安装、更新或卸载操作。npm 随 Node.js 分发；pnpm 不接管来源不明的安装。到左下角切换「系统 / 浅色 / 深色」。
3. 在目录行直接点「改目录」，详情只用于查询命令、配置来源和风险。Go 可以点「批量改目录」，勾选临时目录、模块缓存或构建缓存。输入 `D:\test\go` 会分别预览 `temp`、`mod`、`build` 三个子目录。只更新勾选的当前用户环境变量，不复制、移动或删除旧文件，不修改 GOPATH、系统 TEMP 或 PATH。完成后从资源管理器重开本工具、终端和 IDE，再核对实际路径并做一次构建，通常无需重启电脑。批量配置保留一组撤销记录，先撤销上一批再提交新批；撤销只还原变量，不删除目录。
4. 清理前先退出相关项目与构建、安装进程。预览实际目标和相应命令后再决定；目录结构无法验证、存在目录链接或路径发生变化时拒绝清理。缓存可能需要重新下载。逻辑文件大小不是实际磁盘占用，存在硬链接、扫描不完或同一目录重复时，不能据此判断可回收大小。
5. 安装类操作按确定的包 ID 交给 WinGet，由应用内显示 WinGet 输出与退出状态；不能代替你接受任何协议。操作可能联网、弹出原生安装器或 UAC，并由第三方安装器写入其自身目录。不要把 WinGet 匹配到的软件视为原本一定由 WinGet 安装。npm 随 Node.js 分发；pnpm 的版本管理要看实际安装渠道。

应用文件、WebView2 缓存、外观偏好和回滚数据都跟随 EXE 目录。移动时连同 `DevEnvManagerData` 一起移动；从旧版升级建议解压到新目录，不要拿这版覆盖旧版回滚文件。应用不会重设工具链自身的存储；第三方工具和安装器的写入位置由它们决定。

| 类别 | 查看和变更 | 清理 / 维护 |
|---|---|---|
| npm | 版本、有效缓存、全局前缀和配置；可改缓存变量 | npm 包缓存，不等于 `npx` 缓存 |
| pnpm | 版本、当前项目有效 store 等 | 原生 store prune；不自动改 store，不迁移全局包 |
| Python | Python/pip 版本与路径，pip 缓存和字节码缓存配置 | 仅 pip 缓存；WinGet Python 3.13；不搬动虚拟环境 |
| Go | 版本、缓存/模块/临时目录、工具链配置等 | 仅构建缓存，不清 Go 模块；WinGet Go |
| Node.js | 版本、EXE 路径、受支持版本的编译缓存 | WinGet Node.js LTS；不接管项目产物目录 |

## 构建

Go 1.24+、Node.js 和 npm。Linux/macOS（bash）：`./scripts/build.sh`。它构建前端、生成 Windows 资源并交叉编译 Windows x64 生产版到 `dist/DevEnvManager.exe`。开发机不需要安装 WinForms、PowerShell 或 WebView2 来交叉编译；终端用户运行需要 WebView2。源码包里的 `cmd/window-smoke` 只用于测试，不随运行版分发。

在 Windows 交互桌面上可验证窗口是否真的可见且能正确关闭：
```text
go run ./cmd/window-smoke -exe .\dist\DevEnvManager.exe -title 开发环境管家 -exact -timeout 30s
```
这不会点击管理按钮。源码另附可手动触发的 Windows GitHub Actions 工作流，在构建、Go 测试和可见窗口检查通过后才上传。GitHub 托管运行器不能替代 Windows 10/11 交互桌面验收。完整验证边界见 `TEST-REPORT.md`。

界面参考：[PowerToys Settings](https://learn.microsoft.com/en-us/windows/powertoys/general#settings-search) 的分类和就近搜索，以及 [UniGetUI](https://github.com/Devolutions/UniGetUI#screenshots) 把软件包管理与普通信息分开的做法。

参考：[npm 配置](https://docs.npmjs.com/using-npm/config/) · [pnpm store](https://pnpm.io/cli/store) · [pnpm 跨盘说明](https://pnpm.io/faq) · [Node.js CLI](https://nodejs.org/api/cli.html) · [pip cache](https://pip.pypa.io/en/stable/cli/pip_cache/) · [Python 环境变量](https://docs.python.org/3/using/cmdline.html) · [Go 命令](https://pkg.go.dev/cmd/go) · [WinGet](https://learn.microsoft.com/zh-cn/windows/package-manager/winget/)
