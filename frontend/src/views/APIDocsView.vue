<script setup>
const endpoints = [
  { method: 'GET', path: '/api/status', auth: '无需', description: '查询实例是否已完成首次配置。' },
  { method: 'POST', path: '/api/setup', auth: '无需', description: '首次创建管理员；仅未配置的实例可调用。' },
  { method: 'POST', path: '/api/login', auth: '无需', description: '使用管理员账号登录并建立 Cookie 会话。' },
  { method: 'POST', path: '/api/logout', auth: '必须', description: '注销当前 Cookie 会话。' },
  { method: 'GET', path: '/api/me', auth: '必须', description: '读取当前管理员资料。' },
  { method: 'PUT', path: '/api/me', auth: '必须', description: '更新管理员用户名或密码。' },
  { method: 'GET', path: '/api/settings', auth: '必须', description: '读取上传、安全与 API 设置。' },
  { method: 'PUT', path: '/api/settings', auth: '必须', description: '更新上传、安全与 API 设置。' },
  { method: 'GET', path: '/api/albums', auth: '可选', description: '列出相册；匿名请求的图片数量只统计公开图片。' },
  { method: 'POST', path: '/api/albums', auth: '必须', description: '创建相册。' },
  { method: 'DELETE', path: '/api/albums', auth: '必须', description: '批量删除相册，不删除图片文件。' },
  { method: 'POST', path: '/api/albums/merge', auth: '必须', description: '把多个相册关系合并到目标相册。' },
  { method: 'GET', path: '/api/images', auth: '可选', description: '分页列出图片；匿名请求只返回公开图片。' },
  { method: 'POST', path: '/api/images', auth: '必须', description: '以 multipart/form-data 批量上传图片。' },
  { method: 'DELETE', path: '/api/images', auth: '必须', description: '批量删除图片记录和原文件。' },
  { method: 'PUT', path: '/api/images/visibility', auth: '必须', description: '批量设置图片为公开或私密。' },
  { method: 'POST', path: '/api/images/albums', auth: '必须', description: '批量把图片加入一个或多个相册。' },
  { method: 'DELETE', path: '/api/images/albums', auth: '必须', description: '批量把图片移出一个或多个相册。' },
  { method: 'GET', path: '/api/tokens', auth: '必须', description: '列出 API Token 的名称与标识。' },
  { method: 'POST', path: '/api/tokens', auth: '必须', description: '生成 API Token；完整 Token 仅在此次响应中返回。' },
  { method: 'DELETE', path: '/api/tokens', auth: '必须', description: '批量删除并立即撤销 API Token。' },
  { method: 'GET', path: '/api/tokens/{id}', auth: '必须', description: '读取 Token 的创建时间与最后使用时间。' },
  { method: 'GET', path: '/random', auth: '无需', description: '随机选择一张公开图片，并以 302 重定向到原图地址。' },
  { method: 'GET', path: '/i/{name}', auth: '可选', description: '读取图片原文件；私密图片需要鉴权，匿名请求返回 404。' },
]

const requestBodies = [
  { endpoint: 'POST /api/setup', body: '{ "username": "Admin", "password": "Admin123", "databaseType": "sqlite" }' },
  { endpoint: 'POST /api/login', body: '{ "username": "Admin", "password": "Admin123" }' },
  { endpoint: 'PUT /api/me', body: '{ "username": "Admin", "currentPassword": "旧密码", "newPassword": "新密码" }' },
  { endpoint: 'POST /api/albums', body: '{ "name": "旅行" }' },
  { endpoint: 'DELETE /api/albums', body: '{ "ids": [1, 2] }' },
  { endpoint: 'POST /api/albums/merge', body: '{ "ids": [1, 2], "targetId": 1 }' },
  { endpoint: 'DELETE /api/images', body: '{ "ids": [10, 11] }' },
  { endpoint: 'PUT /api/images/visibility', body: '{ "ids": [10, 11], "isPublic": true }' },
  { endpoint: 'POST 或 DELETE /api/images/albums', body: '{ "imageIds": [10, 11], "albumIds": [1, 2] }' },
  { endpoint: 'POST /api/tokens', body: '{ "name": "自动上传脚本" }' },
  { endpoint: 'DELETE /api/tokens', body: '{ "ids": [1, 2] }' },
]
</script>

<template>
  <section class="content-page api-docs-page">
    <div class="page-heading management-heading">
      <div><span class="eyebrow">API REFERENCE</span><h1>API 文档</h1><p>BlueScale 当前提供的全部 HTTP 端点与鉴权要求。</p></div>
      <RouterLink class="secondary-button header-action" to="/api">返回 API</RouterLink>
    </div>

    <section class="docs-card docs-intro">
      <h2>鉴权方式</h2>
      <p>API 客户端在请求头中携带 <code>Authorization: Bearer &lt;token&gt;</code>。网页端登录 Cookie 同样可通过“必须鉴权”的端点。</p>
      <div class="docs-auth-grid"><div><strong>无需</strong><span>不提供凭据即可调用</span></div><div><strong>可选</strong><span>匿名只能读取公开内容；有效凭据可读取完整内容</span></div><div><strong>必须</strong><span>缺少或使用无效凭据时返回 401</span></div></div>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">ENDPOINTS</span><h2>端点一览</h2></div><span>{{ endpoints.length }} 个端点</span></header>
      <div class="endpoint-table-wrap">
        <table class="endpoint-table">
          <thead><tr><th>方法</th><th>路径</th><th>鉴权</th><th>说明</th></tr></thead>
          <tbody><tr v-for="endpoint in endpoints" :key="`${endpoint.method}-${endpoint.path}`"><td><span class="method-badge" :class="endpoint.method.toLowerCase()">{{ endpoint.method }}</span></td><td><code>{{ endpoint.path }}</code></td><td><span class="auth-badge" :class="{ required: endpoint.auth === '必须', optional: endpoint.auth === '可选' }">{{ endpoint.auth }}</span></td><td>{{ endpoint.description }}</td></tr></tbody>
        </table>
      </div>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">IMAGE LIST</span><h2>图片列表查询参数</h2></div></header>
      <div class="docs-parameter-grid"><div><code>page</code><span>页码，默认 1</span></div><div><code>pageSize</code><span>每页 1–200 张，默认 24</span></div><div><code>format</code><span>jpeg、png、gif、webp、avif 或 all</span></div><div><code>album</code><span>相册 ID，或 none 表示未分类</span></div><div><code>visibility</code><span>public、private 或 all；私密结果需要鉴权</span></div></div>
      <p class="docs-example"><code>GET /api/images?page=1&amp;pageSize=24&amp;format=webp&amp;album=2</code></p>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">RANDOM IMAGE</span><h2>随机图片</h2></div></header>
      <p class="docs-copy"><code>GET /random</code> 返回 <code>302 Found</code>，<code>Location</code> 指向随机选中的公开原图；客户端应允许跟随重定向。重定向本身不缓存，公开原图可由共享 CDN 缓存一天。使用英文逗号分隔多个相册名称；相册名称本身不能包含英文逗号。</p>
      <div class="docs-parameter-grid"><div><code>albums</code><span>可选，相册名称列表，例如 旅行,收藏</span></div><div><code>mode</code><span>可选，union 表示并集，intersection 表示交集；省略时读取系统设置</span></div></div>
      <p class="docs-example"><code>GET /random?albums=album_a,album_b</code></p>
      <p class="docs-example"><code>GET /random?albums=album_c,album_d&amp;mode=intersection</code></p>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">PUBLIC IMAGE CORS</span><h2>公开图片跨域访问</h2></div></header>
      <p class="docs-copy">在“系统设置 → 安全 → 公开图片 CORS”中开启后，<code>/random</code> 和公开的 <code>/i/{name}</code> 会返回 CORS 响应头，并支持 GET、HEAD 的 OPTIONS 预检。允许来源可以设置为 <code>*</code> 或一个完整的 HTTP/HTTPS 来源；私密图片和管理 API 不受影响，也不会发送跨域凭据。</p>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">UPLOAD</span><h2>上传图片</h2></div></header>
      <p class="docs-copy"><code>POST /api/images</code> 使用 <code>multipart/form-data</code>，字段如下：</p>
      <div class="docs-parameter-grid"><div><code>files</code><span>一个或多个图片文件；支持 JPEG、PNG、GIF、WebP、AVIF</span></div><div><code>albumIds</code><span>JSON 数组字符串，例如 [1,2]</span></div><div><code>isPublic</code><span>true 或 false；省略时默认为 false（私密）</span></div></div>
      <pre><code>curl -X POST https://example.com/api/images \
  -H "Authorization: Bearer bsk_..." \
  -F "files=@photo.png" \
  -F "albumIds=[1]" \
  -F "isPublic=false"</code></pre>
    </section>

    <section class="docs-card">
      <header><div><span class="eyebrow">JSON BODIES</span><h2>常用请求体</h2></div></header>
      <div class="request-body-list"><article v-for="item in requestBodies" :key="item.endpoint"><strong>{{ item.endpoint }}</strong><code>{{ item.body }}</code></article></div>
      <p class="docs-copy">更新系统设置时，提交 <code>GET /api/settings</code> 返回的完整对象。除上传接口外，JSON 请求应设置 <code>Content-Type: application/json</code>。</p>
    </section>

    <section class="docs-card docs-status-card">
      <header><div><span class="eyebrow">RESPONSES</span><h2>状态码</h2></div></header>
      <div class="docs-status-grid"><div><strong>200 / 201 / 204</strong><span>请求成功、资源已创建或无响应体</span></div><div><strong>302</strong><span>随机图片已选择，并临时重定向到原图地址</span></div><div><strong>400</strong><span>参数或请求体无效</span></div><div><strong>401</strong><span>缺少或使用无效凭据</span></div><div><strong>404</strong><span>资源不存在；私密图片对匿名请求也返回 404</span></div><div><strong>409</strong><span>重复资源或当前状态冲突</span></div><div><strong>429</strong><span>登录失败次数超过限制</span></div></div>
    </section>
  </section>
</template>
