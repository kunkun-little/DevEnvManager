# 开发环境管家 0.4.1

中文桌面工具，面向 Windows 10/11 x64。采用 Go + Wails v2 + Vue；界面和管理逻辑都在一个 EXE 内，不依赖同目录脚本、不使用 PowerShell 启动。需要 Microsoft Edge WebView2 Runtime；如机器没有，从[微软官方页面](https://developer.microsoft.com/microsoft-edge/webview2/)安装 x64 Evergreen Runtime。应用不修改执行策略，不静默安装运行时。

## 使用

1. 把 ZIP 完整解压到一个普通可写的本地目录，双击 `DevEnvManager.exe`。首次使用先看「空间总览」：C 盘剩余空间与识别出的开发缓存分开显示。
2. 在左侧选择环境，版本栏显示本机版本和上游最新正式版。Node.js 同时显示最新 LTS。发布源按需联网查询，成功结果在内存中保存 30 分钟。更新或卸载前会列出匹配的软件包、命令或原渠道维护入口。
3. 点击目录行的「改目录」，或点击「批量改目录」后在当前列表勾选项目。填写目标并点击「确定」，核对每项的当前目录、设置后目录和旧数据策略，再点击「确认执行」。批量模式在根目录下为每项生成独立子目录，搜索支持变量名。Go 支持 GOPATH 工作区、GOBIN 命令安装位置和三个缓存、临时目录设置。GOENV 文件、Go 安装目录等以「建议保留」标识。
   - **迁移到新目录（默认）** 会把旧数据复制到空目标。旧副本默认保留。对支持原生清理的缓存，可取消「保留旧缓存副本」，所有目录准备完成、设置生效后再清理旧缓存。
   - **仅切换目录** 可以使用已迁好的非空目录，例如已有的 Go 工作区。目标内容保持原样。
   - **删除旧缓存** 会切换到空目录并调用对应工具清理旧缓存。工作区源码、用户安装的命令和系统临时目录受保护。
   - 修改完成后重开终端、IDE 和本工具。撤销恢复原用户变量，新目录里的文件继续保留。已有单项或批量撤销记录时，界面提供对应的撤销入口。
4. 清理前先退出相关项目与构建、安装进程。预览实际目标和相应命令后再决定；目录结构无法验证、存在目录链接或路径发生变化时拒绝清理。缓存可能需要重新下载。逻辑文件大小不是实际磁盘占用，存在硬链接、扫描不完或同一目录重复时，不能据此判断可回收大小。
5. WinGet 维护会核对精确包 ID 和登记版本。可验证的 npm 全局安装可直接通过 npm 更新或卸载。其余安装提供官方更新入口、Windows 已安装的应用或 pnpm 的卸载说明。Node.js 捆绑的 npm 会提示与 Node.js 的关系。安装器可能请求管理员权限。

应用文件、WebView2 缓存、外观偏好和回滚数据都跟随 EXE 目录。移动时连同 `DevEnvManagerData` 一起移动；从旧版升级建议解压到新目录，不要拿这版覆盖旧版回滚文件。应用不会重设工具链自身的存储；第三方工具和安装器的写入位置由它们决定。

| 类别 | 查看和变更 | 清理 / 维护 |
|---|---|---|
| npm | 版本、有效缓存、全局前缀和配置；可单项或批量设置缓存与全局前缀 | npm 包缓存，不等于 `npx` 缓存 |
| pnpm | 版本，以及可修改的存储根目录、元数据缓存、状态目录和全局包目录 | 可单项或批量设置；原生 store prune 只清理未引用内容 |
| Python | Python/pip 版本与路径，pip 缓存和字节码缓存配置 | 仅 pip 缓存；按当前解释器分支匹配 WinGet；不搬动虚拟环境 |
| Go | 版本、缓存/模块/临时目录、工具链配置等 | 常规清理仅支持构建缓存。切换模块目录时可明确选择清理旧模块缓存；WinGet Go |
| Node.js | 版本、EXE 路径、受支持版本的编译缓存 | WinGet Node.js LTS 或正式版 |

目录确认页列出工具将写入的当前用户环境变量。npm、pnpm 的原配置文件保留，撤销会恢复原变量。pnpm 10 使用 NPM_CONFIG_*，从 pnpm 11 起使用 PNPM_CONFIG_*。存储目录填写根目录，例如 D:\Data\pnpm\store。修改 npm 全局前缀后，请核对 PATH 与现有全局命令入口。

## 构建

Go 1.24+、Node.js 和 npm。Linux/macOS（bash）：`./scripts/build.sh`。它构建前端、生成 Windows 资源并交叉编译 Windows x64 生产版到 `dist/DevEnvManager.exe`。开发机不需要安装 WinForms、PowerShell 或 WebView2 来交叉编译；终端用户运行需要 WebView2。源码包里的 `cmd/window-smoke` 只用于测试，不随运行版分发。

在 Windows 交互桌面上可验证窗口是否真的可见且能正确关闭：
```text
go run ./cmd/window-smoke -exe .\dist\DevEnvManager.exe -title 开发环境管家 -exact -timeout 30s
```
这不会点击管理按钮。源码另附可手动触发的 Windows GitHub Actions 工作流，在构建、Go 测试和可见窗口检查通过后才上传。GitHub 托管运行器不能替代 Windows 10/11 交互桌面验收。完整验证边界见 `TEST-REPORT.md`。

界面参考：[PowerToys Settings](https://learn.microsoft.com/en-us/windows/powertoys/general#settings-search) 的分类和就近搜索，以及 [UniGetUI](https://github.com/Devolutions/UniGetUI#screenshots) 把软件包管理与普通信息分开的做法。

参考：[npm 配置](https://docs.npmjs.com/using-npm/config/) · [pnpm store](https://pnpm.io/cli/store) · [pnpm 跨盘说明](https://pnpm.io/faq) · [Node.js CLI](https://nodejs.org/api/cli.html) · [pip cache](https://pip.pypa.io/en/stable/cli/pip_cache/) · [Python 环境变量](https://docs.python.org/3/using/cmdline.html) · [Go 命令](https://pkg.go.dev/cmd/go) · [WinGet](https://learn.microsoft.com/zh-cn/windows/package-manager/winget/)
