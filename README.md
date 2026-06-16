# AriaMX

带认证功能的 aria2 Web 面板。后端使用 Go 标准库，前端使用 Vue 3 + TypeScript，构建后的前端资源会嵌入 Go 二进制。

## 功能

- 登录认证和服务端 Cookie 会话
- aria2 all-in-one 托管：面板内置 aria2 运行时，自动拉起本地 RPC
- 面板层 JSON-RPC 代理：统一通过 `/jsonrpc` 暴露 HTTP / WS 接口；外部调用使用面板独立 RPC Secret，不复用 aria2 内部 Secret；在 TLS 入口下可直接使用 HTTPS / WSS
- 下载总览、任务分类、搜索、排序、任务详情
- 任务暂停、强制暂停、继续、移除、清理结果、队列置顶/置底
- 全局暂停、全局继续、清理下载结果、保存 aria2 session
- URL、磁力链接、种子文件创建任务
- 新建任务高级参数：目录、输出文件名、连接数、分片数、限速、Header、BT 做种参数
- 任务详情会按当前任务实际内容展示有意义的标签页，例如概览、文件、Peer、Server、Tracker；没有数据的页签不会显示
- BT 多文件选择，单任务选项查看和修改
- aria2 全局选项按 AriaNg 分组查看和修改，包含中文名称、说明、只读状态和常用枚举选择
- 全局选项保存、重置默认值，以及“需要重启才生效”的选项自动重启 aria2
- AIO 模式下，若内置 RPC 端口冲突，会按 `+10` 自动步进到下一个可用端口并回写配置
- AIO 模式下，面板收到 `SIGINT` / `SIGTERM` 时会主动停止内置 aria2
- 节点防护：扫描活动 BT 任务的 Peer，按评分识别高风险节点，并通过系统防火墙封禁 IP
- 左侧栏显示面板版本和当前 aria2 版本
- 刷新间隔、默认下载目录、浅色/深色模式和面板密码设置
- 按平台单二进制部署：每个发布产物只内嵌对应平台的 aria2 运行时

## 开发

```bash
pnpm install
ARIAMX_ADDR=127.0.0.1:18081 ARIAMX_CONFIG=ariamx.json go run ./cmd/ariamx
pnpm run dev
```

开发模式下 Vite 会把 `/api` 代理到 `http://127.0.0.1:18081`。如果默认端口被占用，Vite 会自动切换到下一个可用端口。

## 构建

```bash
pnpm run build:allinone
```

这会为当前目标平台构建 all-in-one 版本。默认只准备并内嵌当前 `GOOS/GOARCH` 对应的 aria2 运行时，不再把 `darwin-arm64`、`linux-amd64`、`linux-arm64` 三套运行时一起塞进同一个二进制。

本地构建产物：

```text
dist/ariamx
```

如果需要预生成全部运行时归档，再执行：

```bash
pnpm run prepare:aria2:all
```

## GitLab CI 发布

仓库内提供了 `.gitlab-ci.yml`，会按矩阵分别构建：

- `linux-amd64`
- `linux-arm64`
- `darwin-arm64`

每个 CI 产物都只内嵌自身平台所需的 aria2 运行时，因此发布包不会再出现一个 `93M` 的“全平台合体版”。

## 运行

```bash
ARIAMX_ADDR=:8080 \
ARIAMX_CONFIG=ariamx.json \
ARIAMX_ADMIN_PASSWORD='change-me' \
./dist/ariamx
```

首次启动会创建 `ARIAMX_CONFIG` 指定的配置文件。默认用户名是 `admin`；建议首次启动显式设置 `ARIAMX_ADMIN_PASSWORD`。

默认情况下，AriaMX 会在当前配置目录旁创建 `ariamx-data/aria2/`，自动释放并启动内置 aria2。只有在你显式设置 `ARIAMX_ARIA2_RPC` 时，才会切回外部 aria2 RPC 模式。

如果内置 aria2 的目标 RPC 端口已被其他程序占用，AIO 会从当前目标端口开始按 `+10` 递增寻找空闲端口，并把最终端口写回 `ariamx.json`。例如默认 `16800` 被占用时，会依次尝试 `16810`、`16820`。

登录后，面板层 RPC 代理路径固定为：

```text
/jsonrpc
```

该路径支持标准 JSON-RPC POST，也支持 WebSocket 升级。面板自身当前仍以 HTTP 提供服务；如果前面挂了 TLS 反向代理，那么同一路径即可直接以 `https://.../jsonrpc` 和 `wss://.../jsonrpc` 方式访问。

外部程序调用 `/jsonrpc` 时，默认按 aria2 原生习惯传递面板层 RPC Secret，也就是把它放进 JSON-RPC `params[0]`：

```text
{"jsonrpc":"2.0","id":"1","method":"aria2.getVersion","params":["token:<panel rpc secret>"]}
```

该 Secret 保存在 `ariamx.json` 的 `panel.rpcSecret`，与 aria2 内部使用的 `aria2.rpcSecret` 始终独立。AIO 模式下内置 aria2 只监听 `127.0.0.1`，外部不能直接绕过面板访问它。

登录后的“连接信息”页面会直接显示 HTTP / WS 连接方式以及当前面板 RPC Secret。

为了兼容不同客户端，面板 `/jsonrpc` 仍然兼容 `Authorization: Bearer <panel rpc secret>` 和 `ws://.../jsonrpc?secret=<panel rpc secret>` 这两种入口；但默认文档和面板页面都按 aria2 / AriaNg 更常见的 `token:<secret>` 习惯展示。面板会先校验这个 token 是否等于 `panel.rpcSecret`，只有校验通过后才会继续转发，并在转发到 aria2 前自动替换成内部 `aria2.rpcSecret`。如果填写的是错误的面板 secret，调用仍然会被拒绝。

另外，面板还提供可开关的 MCP HTTP 入口：

```text
/mcp
```

该入口使用同一个 `panel.rpcSecret` 做鉴权，支持 `Authorization: Bearer <panel rpc secret>`、`X-AriaMX-Secret` 和 `?secret=<panel rpc secret>`。当前 MCP 按 `2024-11-05` 协议版本提供完整的服务端原语：

- `initialize`
- `ping`
- `tools/list`
- `tools/call`
- `resources/list`
- `resources/read`
- `resources/templates/list`
- `prompts/list`
- `prompts/get`
- `completion/complete`

其中：

- `tools/*` 暴露一组 aria2 操作能力，例如 `aria2_get_version`、`aria2_tell_active`、`aria2_add_uri`、`aria2_pause`、`aria2_remove`
- `resources/*` 暴露连接信息、面板配置、aria2 全局配置、全局统计以及任务列表和单任务详情
- `prompts/*` 提供失败任务诊断、下载队列总结、客户端连接说明和 aria2 参数调优等 Prompt 模板
- `completion/complete` 为 Prompt 参数和资源模板参数提供补全，例如任务 `gid`、任务分桶 `bucket` 和 aria2 配置项 `key`

登录后的“连接信息”页面会展示 MCP 地址与初始化请求；“MCP”页面只展示可用工具列表。若在“面板设置”里关闭 MCP，`/mcp` 会直接拒绝访问。

登录后可在“设置”页切换 `AriaMX` 皮肤的 `跟随系统` / `浅色` / `深色` 模式。显示模式会写入 `ariamx.json` 的 `panel.theme` / `panel.colorMode`，后续登录会继续沿用；选择“跟随系统”后，系统深浅色变化会实时同步到面板。

如果直链任务因为旧的 `.aria2` 控制文件与当前分片信息冲突而失败，面板会直接提示这是续传控制文件冲突；对失败任务点击“重新开始”时，面板会自动挪开冲突的 `.aria2` 控制文件后再重建任务。

任务列表中的“移除”现在区分两种情况：

- 未完成任务：先从 aria2 队列中移除，再直接删除对应文件和 `.aria2` 控制文件
- 已完成任务：只移除 aria2 历史记录，保留已下载文件

“节点防护”页面会扫描当前活动 BT 任务的 Peer，并按下载/上传行为打分。只有达到评分阈值的节点才会进入自动封禁逻辑，避免“看到可疑节点就直接封”的误判。当前默认自动封禁阈值是 `3` 分，可通过页面开关启用或关闭自动封禁。

封禁依赖系统防火墙：

- macOS：优先使用 `pf`
- Linux：按系统已存在且可操作的防火墙自动选择，支持 `firewalld`、`ufw`、`nftables`、`iptables`

只有规则成功应用后，节点才会被正式加入封禁名单。若当前进程没有足够权限，节点防护页面会显示遮罩并禁止操作，同时明确提示需要修复系统防火墙权限；此时节点不会被错误地显示为已封禁。

## 验证

```bash
pnpm run lint
pnpm run check
go test ./...
pnpm run build:all
```
