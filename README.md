# AriaMX

带认证功能的 aria2 Web 面板。后端使用 Go 标准库，前端使用 Vue 3 + TypeScript，构建后的前端资源会嵌入 Go 二进制。

## 功能

- 登录认证和服务端 Cookie 会话
- aria2 JSON-RPC 后端代理，RPC Secret 不暴露到浏览器
- 下载总览、任务分类、搜索、排序、任务详情
- 任务暂停、强制暂停、继续、移除、清理结果、队列置顶/置底
- 全局暂停、全局继续、清理下载结果、保存 aria2 session
- URL、磁力链接、种子文件创建任务
- 新建任务高级参数：目录、输出文件名、连接数、分片数、限速、Header、暂停创建、BT 做种参数
- 任务详情标签页：概览、文件、选项、Peer、Server、Tracker
- BT 多文件选择，单任务选项查看和修改
- aria2 全局选项查看和修改
- aria2 RPC 地址、Secret、刷新间隔、默认下载目录和面板密码设置
- 单二进制部署：`dist/ariamx`

## 开发

```bash
pnpm install
ARIAMX_ADDR=127.0.0.1:18081 ARIAMX_CONFIG=ariamx.json go run ./cmd/ariamx
pnpm run dev
```

开发模式下 Vite 会把 `/api` 代理到 `http://127.0.0.1:18081`。如果默认端口被占用，Vite 会自动切换到下一个可用端口。

## 构建

```bash
pnpm run build:all
```

构建产物：

```text
dist/ariamx
```

## 运行

```bash
ARIAMX_ADDR=:8080 \
ARIAMX_CONFIG=ariamx.json \
ARIAMX_ADMIN_PASSWORD='change-me' \
ARIAMX_ARIA2_RPC='http://127.0.0.1:6800/jsonrpc' \
ARIAMX_ARIA2_SECRET='' \
./dist/ariamx
```

首次启动会创建 `ARIAMX_CONFIG` 指定的配置文件。默认用户名是 `admin`；建议首次启动显式设置 `ARIAMX_ADMIN_PASSWORD`。

## 验证

```bash
pnpm run lint
pnpm run check
go test ./...
pnpm run build:all
```
