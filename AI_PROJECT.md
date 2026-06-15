# AI 项目经验

- 症状：生产构建后的页面出现模板自带外部跳转入口，浏览器验证时会离开本地面板。原因：Vite 模板默认启用了 `vite-plugin-trae-solo-badge`。预防：面向实际发布的 Web 面板必须移除模板广告/徽标插件，并检查 `index.html` 标题与语言。
- 症状：本地冒烟测试访问 `127.0.0.1:18080` 命中了其他进程。原因：已有 IPv4 监听进程占用同端口，而测试服务监听在 IPv6 通配地址。预防：冒烟测试使用明确的 `127.0.0.1:<port>` 绑定，并用未占用端口。
- 症状：`.gitignore` 中写入 `ariamx` 会把源码目录 `cmd/ariamx/` 一并忽略。原因：无斜杠的 gitignore 规则会匹配任意层级同名文件或目录。预防：忽略根目录二进制时使用 `/ariamx`。
- 症状：冒烟验证中设置 `ARIAMX_ADMIN_PASSWORD=admin` 后仍无法登录。原因：`ARIAMX_ADMIN_PASSWORD` 只在配置文件首次创建时生效，已存在配置不会覆盖密码。预防：认证相关冒烟测试使用全新的 `ARIAMX_CONFIG` 路径。
- 症状：Vite 开发环境打开页面后 `/api` 请求会打到前端端口。原因：开发模式没有把 `/api` 转发给 Go 后端。预防：`vite.config.ts` 中配置 `/api` 代理到本地 Go 服务端口，并在 README 记录 dev 启动顺序。
- 症状：aria2 设置页只有裸 `key=value`，用户难以对应 AriaNg 的设置入口。原因：只读取了 `aria2.getGlobalOption`，没有维护 AriaNg 的分类和中文文案元数据。预防：全局选项页保留 AriaNg 的基本、HTTP/FTP/SFTP、HTTP、FTP/SFTP、BitTorrent、Metalink、RPC、高级分组，并为常用项提供中文名称、说明、只读和枚举信息。
- 症状：aria2 设置页新增高级 `key=value` 编辑区后 UI 被撑乱。原因：raw textarea 常驻在设置页下方，且 `.options-panel textarea` 全局选择器会影响该面板内所有 textarea。预防：高级编辑区默认折叠，并用 `.raw-option-editor textarea` 精确限定大文本框样式。
- 症状：aria2 设置页右侧标题在两栏结构外，右侧内容上下边界无法和左侧分类栏对齐。原因：标题和操作按钮不在 `.settings-layout` 同一网格内。预防：设置页标题、搜索和操作按钮都放进右侧 `.settings-content`，两栏容器只负责左右对齐。
- 症状：用户要求界面“像 AriaNg”，但局部组件调整后整体观感仍停留在玻璃拟态。原因：主题色、边框、按钮、列表和侧栏基线样式都集中在 `src/style.css`，只改单个组件无法完成整体迁移。预防：涉及整站风格切换时，优先统一全局设计 token 与共享容器/控件样式，再处理页面细节。
- 症状：用户提供了设计导出目录后，若只参考单个 HTML 页面，容易漏掉统一的颜色、字号和间距规范。原因：结构示例、设计 token 和说明分别散落在 `ariang-config-design.html`、`ariang-admin-style.css`、`ariang-admin-style-guide.md`。预防：遇到该目录下的样式对齐任务时，优先以 CSS 和 style guide 作为全局视觉基线，再用 HTML 对齐局部结构。
- 症状：用户要求“经典皮肤 / 设计稿皮肤”按账号保留时，只靠 `localStorage` 会在清缓存或换设备后丢失。原因：皮肤状态属于面板配置，不是纯浏览器临时偏好。预防：皮肤字段写入 `panel.theme` 并通过 `/api/config` 下发；前端启动和登录成功后都要重新应用服务端主题。
- 症状：all-in-one 模式启动内置 aria2 时，若运行时目录用相对路径或未预创建 `session.dat`，`aria2c` 会直接启动失败。原因：进程执行路径依赖当前工作目录，且 `--input-file` 指向的 session 文件必须存在。预防：内置 aria2 的状态目录统一转成绝对路径，并在首次启动前创建空的 `session.dat`。
- 症状：all-in-one 发布包体积异常膨胀到约 93M。原因：`//go:embed runtime/*` 会把 `darwin-arm64`、`linux-amd64`、`linux-arm64` 三套 aria2 运行时归档全部打进每一个二进制。预防：all-in-one 构建必须使用按 `GOOS/GOARCH` 拆分的 build tag embed，并让 CI 按平台分别产出发布包。
- 症状：面板里修改 `rpc-listen-port` 后界面显示已保存，但内置 aria2 实际仍监听旧端口。原因：Aria2 选项里的 `rpc-listen-port` 与面板托管字段 `ManagedRPCPort` 是两套状态，保存时没有同步。预防：all-in-one 模式下凡是由面板启动参数接管的 aria2 选项，保存时必须先映射回托管配置，再决定是否重启 aria2。
