# HTTPX Debugger

Windows 本机 HTTP/HTTPS 抓包工具（自研 MVP）。

- **`core/`** — Go 编写的代理核心：本地代理 + HTTPS MITM 解密 + 会话记录 + 控制/事件服务
- **`ui/`** — Electron + Vue 3 桌面界面（技术栈参考 `G:\git_project\aigc-ui`：electron-vite + Vue 3 + Pinia + TS）

```
┌──────────────────────────────────────────────┐
│  UI（Electron + Vue3）                        │
│  会话表格 | 请求/响应详情 | 过滤 | 导出 HAR      │
└──────────────▲───────────────────────────────┘
               │ TCP JSON 行协议（127.0.0.1:8900）
┌──────────────┴───────────────────────────────┐
│  core/httpx-core.exe（Go）                    │
│  本地代理 127.0.0.1:8899 + MITM + 会话记录     │
└──────────────────────────────────────────────┘
```

## 原理（简要）

1. **拦截**：core 起一个本地 HTTP 代理（`127.0.0.1:8899`），并把它设为本机系统代理（改 WinINET 注册表 + 广播刷新）。走系统代理的程序流量都经过它。
2. **解密**：HTTPS 场景，客户端先发 `CONNECT host:443`，core 对客户端扮演 TLS 服务端——出示一张由自签根证书签发的、目标域名的叶子证书（根证书需用户信任，UI 一键安装到当前用户 ROOT 库）。解密后 core 与真实服务器再建一条 TLS 转发，两侧明文都记录。
3. **呈现**：每个请求/响应对序列化为结构化 JSON，经 TCP 事件流推给 UI；UI 展示会话表格与详情，可过滤、导出 HAR。

不依赖任何抓包驱动；对「不走系统代理」的程序（原生 socket、部分游戏/客户端）抓不到——这是 MVP 的已知边界，进阶方案（WinDivert 内核重定向）见 README 末尾。

## 构建与启动

前置：Go 1.24+、Node 18+。

**一键启动**（推荐，自动构建缺失部分并拉起 UI）：

```powershell
start.bat
```

或分步手动：

```powershell
# 1. 构建 Go 核心
cd core
go build -o httpx-core.exe .

# 2. 安装 UI 依赖并启动
cd ..\ui
npm install
npm run dev        # 开发模式（自动拉起 core）
```

> `start.bat` 会：检查并重建过期的 `core\httpx-core.exe` → 补装 UI 依赖 → 以开发模式启动。设环境变量 `HTTPX_DRY=1` 可只做检查不启动。

启动后：

1. 点「安装证书」信任根证书（HTTPS 明文必需；只装当前用户 ROOT 库，免管理员）。
2. 点「开始捕获」——**自动接管系统代理**，浏览器/桌面应用流量即被记录。
3. 正常使用浏览器/程序。
4. 用完点「停止捕获」（**自动还原系统代理**，含你原有的代理配置），或直接关窗口（退出自动清理）。

> 系统代理接管与还原是全自动的：开始捕获时写入 `ProxyEnable=1/ProxyServer=127.0.0.1:8899`，
> 停止捕获时还原为抓包前的原始值（包括你之前配置的公司代理）。抓终端/原生程序用「全流量」模式。

> 证书文件生成在 `%APPDATA%\httpx-debugger-ui\ca\`（`ca.pem` / `ca-key.pem`）。也可命令行管理：
>
> ```powershell
> core\httpx-core.exe ca --data-dir data           # 查看/生成证书
> core\httpx-core.exe ca --data-dir data --install # 安装到当前用户 ROOT 库
> ```

## 项目结构

```
httpx_debugger/
├── core/                     # Go 代理核心（纯标准库 + x/sys + brotli + godivert）
│   ├── main.go               # serve / ca 子命令
│   ├── ca.go                 # 根 CA 生成、每域名叶子证书签发（缓存）
│   ├── proxy.go              # HTTP 代理 + CONNECT MITM + 记录 + 拦截钩子 + 重放
│   ├── intercept.go          # 拦截器：命中规则暂停请求/响应，等待 UI 裁决，超时放行
│   ├── redirect.go           # WinDivert 重定向引擎（双向改包 + 映射表 + 白名单防环路）
│   ├── transparent.go        # 透明连接处理（TLS SNI / HTTP Host / 隧道）
│   ├── tls_sni.go            # ClientHello SNI 解析（peek 不消费）
│   ├── body.go               # body 读取上限、gzip/deflate/br 解压
│   ├── session.go            # Session 结构与内存环形缓冲
│   ├── server.go             # 控制命令 + 事件广播（TCP JSON 行协议）
│   └── sysproxy_windows.go   # 系统代理开关、证书安装/检测（certutil）
├── scripts/
│   ├── test-control.mjs      # 核心冒烟测试（curl 经代理 + 事件断言）
│   ├── test-echo-server.mjs  # 本地回显服务器（验证拦截修改是否真正生效）
│   ├── test-intercept.mjs    # 断点改包 + 重放协议测试（23 项断言）
│   ├── test-envproxy.mjs     # 环境变量代理验证（终端 CLI 抓包）
│   ├── e2e-driver.mjs        # 应用级 E2E 驱动（自动放行拦截）
│   ├── setup-windivert.ps1   # 下载并安装 WinDivert 驱动（管理员）
│   └── normalize-eol.ps1     # 文件行尾/编码规范化（UTF-8 + LF）
└── ui/
    ├── electron/main/        # 主进程：拉起 core、TCP 客户端、HAR 保存
    ├── electron/preload/     # contextBridge 暴露 window.api
    └── web/src/              # Vue3 界面：store / 组件 / 工具
```

## 控制协议（TCP 127.0.0.1:8900，JSON 行）

UI → core：

```json
{"id":1,"cmd":"start","listen":"127.0.0.1:8899"}
{"id":2,"cmd":"stop"}
{"id":3,"cmd":"setProxy","port":8899}
{"id":4,"cmd":"unsetProxy"}
{"id":5,"cmd":"installCA"}
{"id":6,"cmd":"clear"}
{"id":7,"cmd":"setIntercept","enabled":true,"pattern":"demo.cn","direction":"both","timeoutMs":60000}
{"id":8,"cmd":"interceptContinue","sessionId":"s000001","request":{"method":"POST","url":"...","headers":[...],"body":"base64"}}
{"id":9,"cmd":"interceptResponseContinue","sessionId":"s000001","response":{"status":200,"statusText":"OK","headers":[...],"body":"base64"}}
{"id":10,"cmd":"interceptDrop","sessionId":"s000001"}
{"id":11,"cmd":"replay","request":{"method":"GET","url":"https://...","headers":[...],"body":""}}
```

core → UI：

```json
{"type":"reply","id":1,"ok":true}
{"type":"status","status":{...}}
{"type":"session","session":{...}}
{"type":"interceptRequest","sessionId":"s000001","request":{...}}
{"type":"interceptResponse","sessionId":"s000001","response":{...}}
```

## 断点改包 / 重放

- 工具栏「拦截」开启后，命中规则（host 包含关键字，可仅请求/仅响应/两者）的流量转发前暂停。
- 弹窗内可编辑 Method/URL/状态码、headers（增删改）、body（文本可编辑，二进制只读），然后「继续并应用修改」「原样继续」「丢弃」。
- 未裁决的拦截 **60s 自动放行**原包，避免 UI 失联导致请求挂死。
- 会话右键或详情页「重放」：把请求原样重新发送（记录为新会话，不过拦截）。
- 会话右键/详情页可「复制为 cURL」「复制 URL」。

## 全流量抓取（WinDivert，M4）

浏览器/尊重系统代理的程序走「系统代理」即可；**终端、原生程序（不走系统代理的）需要「全流量」模式**：

1. 用**管理员身份**运行 `scripts\setup-windivert.ps1`（下载 WinDivert.dll/.sys 到 `core\`，注册驱动服务）。
2. **以管理员身份启动** HTTPX Debugger（`start.bat` 右键 → 以管理员身份运行），点「开始捕获」→「全流量」。
3. 此时所有出站 TCP 被透明重定向到本地：明文 HTTP 经 Host 头、HTTPS 经 SNI 识别目标，走同一 MITM 管线；未知协议纯隧道转发。

原理：WinDivert 内核驱动双向改包（出站目的改到透明端口 8897，回程源还原为原始目标），4-tuple 映射表维护原始目标，代理自身连接白名单防环路。

**已知限制**：
- 仅 IPv4 TCP；UDP/QUIC 不处理。
- 代理出站连接注册存在极小竞态窗口（偶发连接失败，重试即可）。
- 证书固定（pinning）的应用 HTTPS 仍无法解密（MITM 固有限制）。
- 驱动加载需要管理员权限（UAC），杀软可能提示放行。

## 已知限制与后续路线

- HTTP/2 上游被强制降级为 HTTP/1.1（`TLSNextProto` 置空），保证明文可解析。
- 证书固定（pinning）的应用 HTTPS 会握手失败，属 MITM 固有限制。
- Firefox 用自带证书库，需手动信任 `ca.pem`。
- 未做：UDP/QUIC 抓取、会话持久化（重启丢失）、WebSocket 帧解析、HTTP/2 明文解析、大数据量虚拟滚动。
