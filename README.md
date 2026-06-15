# AriaMX

带认证功能的 aria2 Web 面板。后端使用 Go 标准库，前端使用 Vue 3 + TypeScript，构建后的前端资源会嵌入 Go 二进制。

## 功能

- 登录认证和服务端 Cookie 会话
- aria2 all-in-one 托管：面板内置 aria2 运行时，自动拉起本地 RPC
- 面板层 JSON-RPC 代理：统一通过 `/jsonrpc` 暴露 HTTP / WS 接口；在 TLS 入口下可直接使用 HTTPS / WSS
- 下载总览、任务分类、搜索、排序、任务详情
- 任务暂停、强制暂停、继续、移除、清理结果、队列置顶/置底
- 全局暂停、全局继续、清理下载结果、保存 aria2 session
- URL、磁力链接、种子文件创建任务
- 新建任务高级参数：目录、输出文件名、连接数、分片数、限速、Header、暂停创建、BT 做种参数
- 任务详情标签页：概览、文件、选项、Peer、Server、Tracker
- BT 多文件选择，单任务选项查看和修改
- aria2 全局选项按 AriaNg 分组查看和修改，包含中文名称、说明、只读状态和常用枚举选择
- 全局选项保存、重置默认值，以及“需要重启才生效”的选项自动重启 aria2
- 左侧栏显示面板版本和当前 aria2 版本
- 刷新间隔、默认下载目录、界面皮肤和面板密码设置
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

登录后，面板层 RPC 代理路径固定为：

```text
/jsonrpc
```

该路径支持标准 JSON-RPC POST，也支持 WebSocket 升级。面板自身当前仍以 HTTP 提供服务；如果前面挂了 TLS 反向代理，那么同一路径即可直接以 `https://.../jsonrpc` 和 `wss://.../jsonrpc` 方式访问。

登录后可在“设置”页切换 `经典皮肤` 和 `设计稿皮肤`。皮肤会写入 `ariamx.json` 的 `panel.theme`，后续登录会继续沿用。

## 验证

```bash
pnpm run lint
pnpm run check
go test ./...
pnpm run build:all
```
