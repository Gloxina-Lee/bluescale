# BlueScale

BlueScale 是一个单用户、自托管的图床。前端使用 Vue 3，后端使用 Go，元数据存储在 SQLite；上传的图片由 Go 在 `/i/` 路径直接响应，不依赖 Nginx、Apache 等 Web 服务器提供图片文件。

## 当前功能

- 首次运行配置管理员用户名、密码与数据库类型（当前仅 SQLite）
- 单管理员登录与 HttpOnly Cookie 会话，会话持久化在 SQLite 中，服务重启后仍然有效
- 个人设置支持修改用户名与密码
- 可从账户菜单切换并持久保存浅色或深色模式
- 拖拽或文件资源管理器选择多张图片，确认后点击上传图标开始上传
- 支持 JPG、PNG、GIF、WebP、AVIF；默认单张最大 50 MB、单次最多 50 张，可在系统设置中调整
- 系统设置支持统一转换图片格式、UUIDv4/UUIDv5 或原文件名存储，以及登录失败次数限制和反向代理真实 IP 标头
- 图片管理支持按格式和相册筛选、分页与每页数量设置，以及网格和带小缩略图的列表视图
- 点击图片会在站内预览器中打开，可前后切换、复制公开链接或另行打开原图
- `/random` 可从全部公开图片或指定相册的并集/交集中随机返回图片，多相册默认操作可在系统设置中调整
- 相册采用标签式的多对多关系：同一张图片可加入多个相册而不产生文件副本；支持批量创建、删除、合并和调整图片归属
- 上传时可一次选择多个目标相册；不选择时图片保持未分类
- SQLite WAL 模式；管理员密码使用 bcrypt 哈希保存
- Vue 构建产物嵌入 Go，可部署为单个可执行文件

## 快速开始

环境要求：Go 1.25+、Node.js 20.19+ 或 22.12+。

```powershell
cd frontend
npm install
npm run build
cd ..
go run .
```

浏览器打开 `http://localhost:8080`。首次访问会自动进入首次配置页面。

默认数据目录是程序工作目录下的 `data/`：

```text
data/
├── bluescale.db
└── images/
```

从多用户版本升级时，原用户目录中的图片会自动合并回 `images/`，公开图片链接保持不变；最早创建的管理员账号会成为唯一管理员账号。

可用环境变量：

- `BLUESCALE_ADDR`：监听地址，默认 `:8080`
- `BLUESCALE_DATA_DIR`：SQLite 和图片数据目录，默认 `data`

例如：

```powershell
$env:BLUESCALE_ADDR = "127.0.0.1:9000"
$env:BLUESCALE_DATA_DIR = "D:\BlueScaleData"
go run .
```

## 管理员本地恢复

忘记用户名或密码时，可以直接在 BlueScale 所在主机上恢复管理员账号。恢复前必须先停止正在使用同一数据目录的 BlueScale 服务；程序会通过数据目录锁阻止并发修改。

```bash
# 查看用户名
./bluescale admin show-username --data-dir /var/lib/bluescale

# 修改用户名；执行前会显示数据库路径并要求输入 yes 确认
./bluescale admin reset-username --data-dir /var/lib/bluescale NewAdmin

# 交互式重置密码；输入内容不会显示在终端中
./bluescale admin reset-password --data-dir /var/lib/bluescale

# 如果怀疑凭据已经泄露，同时撤销所有 API Token
./bluescale admin reset-password --data-dir /var/lib/bluescale --revoke-api-tokens
```

用户名或密码重置后，已有网页登录会话都会失效。自动化环境可以使用 `--password-stdin --yes` 从标准输入读取一行新密码；不要把密码写在命令行参数中，否则可能进入 shell 历史或进程列表。数据目录、数据库和锁文件在支持 Unix 权限的平台上会分别收紧为 `0700`、`0600` 和 `0600`。

## 开发模式

先启动 Go API：

```powershell
go run .
```

再启动 Vite（`/api` 和 `/i` 会代理到 `localhost:8080`）：

```powershell
cd frontend
npm run dev
```

访问 `http://localhost:5173`。

## 构建与测试

```powershell
cd frontend
npm run build
cd ..
go test ./...
go build -o bluescale.exe .
```

构建 Linux amd64 单文件版本：

```bash
cd frontend
npm ci
npm run build
cd ..
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/bluescale-linux-amd64 .
```

生产环境若通过 HTTPS 反向代理运行，请先在“系统设置 → 安全”中开启反向代理模式，并传递 `X-Forwarded-Proto: https`；Go 会据此为会话 Cookie 启用 `Secure` 属性。只有在反向代理模式开启后，BlueScale 才会信任所选的真实 IP 标头。图片本身始终由 BlueScale 的 Go 进程从 `/i/` 直出。

生产部署建议使用独立的非 root 系统用户运行 BlueScale，并把 `BLUESCALE_DATA_DIR` 指向仅该用户可访问的本地目录。若前面有反向代理，尽量将 `BLUESCALE_ADDR` 绑定为 `127.0.0.1:8080`（或仅代理可访问的内网地址），由代理终止 HTTPS、覆盖而不是追加真实 IP 标头，并阻止公网绕过代理直连。首次初始化应在开放公网访问前完成，同时定期备份数据库和 `images` 目录。
