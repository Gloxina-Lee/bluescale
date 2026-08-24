# BlueScale

BlueScale 是一个支持多用户与权限分组的自托管图床。前端使用 Vue 3，后端使用 Go，元数据存储在 SQLite；上传的图片由 Go 在 `/i/` 路径直接响应，不依赖 Nginx、Apache 等 Web 服务器提供图片文件。

## 当前功能

- 首次运行配置管理员名称、账号、密码与数据库类型（当前仅 SQLite）
- 多用户登录与 HttpOnly Cookie 会话
- 用户及用户组管理，可分别授予上传图片、管理图片和管理用户权限
- 内置 Admin 与 User 用户组，首次管理员自动归入 Admin 组
- 个人设置支持修改昵称、登录账号与密码
- 可从账户菜单切换并持久保存浅色或深色模式
- 拖拽或文件资源管理器选择多张图片，确认后点击上传图标开始上传
- 支持 JPG、PNG、GIF、WebP、AVIF，单张最大 25 MB，单次最多 50 张
- 图片管理、复制公开链接、全选和批量删除
- SQLite WAL 模式；所有用户密码均使用 bcrypt 哈希保存
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

可用环境变量：

- `BLUESCALE_ADDR`：监听地址，默认 `:8080`
- `BLUESCALE_DATA_DIR`：SQLite 和图片数据目录，默认 `data`

例如：

```powershell
$env:BLUESCALE_ADDR = "127.0.0.1:9000"
$env:BLUESCALE_DATA_DIR = "D:\BlueScaleData"
go run .
```

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

生产环境若通过 HTTPS 反向代理运行，请传递 `X-Forwarded-Proto: https`，Go 会据此为会话 Cookie 启用 `Secure` 属性。图片本身始终由 BlueScale 的 Go 进程从 `/i/` 直出。
