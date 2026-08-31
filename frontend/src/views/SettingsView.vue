<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import BaseSelect from '../components/BaseSelect.vue'
import ErrorToast from '../components/ErrorToast.vue'

const router = useRouter()
const activeTab = ref('upload')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const saved = ref(false)
const form = ref(null)

const targetFormatOptions = [
  { value: 'jpeg', label: 'JPEG' },
  { value: 'png', label: 'PNG' },
  { value: 'webp', label: 'WebP' },
  { value: 'avif', label: 'AVIF' },
]
const renameMethodOptions = [
  { value: 'uuid_v4', label: 'UUIDv4（随机，推荐）' },
  { value: 'uuid_v5', label: 'UUIDv5（基于图片内容）' },
]
const realIPHeaderOptions = [
  { value: 'X-Real-IP', label: 'X-Real-IP' },
  { value: 'X-Forwarded-For', label: 'X-Forwarded-For' },
  { value: 'CF-Connecting-IP', label: 'CF-Connecting-IP' },
]
const albumModeOptions = [
  { value: 'union', label: '并集' },
  { value: 'intersection', label: '交集' },
]

const supportsQuality = computed(() => form.value?.upload.targetImageFormat !== 'png')

async function loadSettings() {
  loading.value = true
  error.value = ''
  try {
    form.value = await api('/api/settings')
  } catch (requestError) {
    error.value = requestError.message
    if (requestError.status === 401) router.push({ name: 'login' })
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  if (!form.value || saving.value) return
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    form.value = await api('/api/settings', { method: 'PUT', body: JSON.stringify(form.value) })
    saved.value = true
    window.setTimeout(() => { saved.value = false }, 2600)
  } catch (requestError) {
    error.value = requestError.message
    if (requestError.status === 401) router.push({ name: 'login' })
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>

<template>
  <section class="content-page settings-page">
    <ErrorToast :message="error" @close="error = ''" />
    <div class="page-heading settings-heading">
      <div>
        <span class="eyebrow">SYSTEM PREFERENCES</span>
        <h1>系统设置</h1>
        <p>调整上传流程、API 行为和服务的安全边界。</p>
      </div>
      <button class="primary-button settings-save-button" type="button" aria-label="保存设置" :disabled="loading || saving" @click="saveSettings">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 4h12l2 2v14H5V4Z"/><path d="M8 4v6h8V4M8 20v-6h8v6"/></svg>
        <span>{{ saving ? '正在保存' : '保存设置' }}</span>
      </button>
    </div>

    <div class="settings-layout">
      <nav class="settings-tabs" aria-label="设置分类">
        <button type="button" :class="{ active: activeTab === 'upload' }" @click="activeTab = 'upload'">
          <svg viewBox="0 0 24 24"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 14v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4"/></svg>
          <span><strong>上传</strong><small>限额、转换与命名</small></span>
        </button>
        <button type="button" :class="{ active: activeTab === 'security' }" @click="activeTab = 'security'">
          <svg viewBox="0 0 24 24"><path d="M12 3 5 6v5c0 4.7 2.9 8.2 7 10 4.1-1.8 7-5.3 7-10V6l-7-3Z"/><path d="m9 12 2 2 4-4"/></svg>
          <span><strong>安全</strong><small>登录、跨域与代理</small></span>
        </button>
        <button type="button" :class="{ active: activeTab === 'api' }" @click="activeTab = 'api'">
          <svg viewBox="0 0 24 24"><path d="M8 9 5 12l3 3m8-6 3 3-3 3M14 5l-4 14"/></svg>
          <span><strong>API</strong><small>随机图片接口</small></span>
        </button>
      </nav>

      <div class="settings-content">
        <div v-if="loading" class="loading-state"><span></span><p>正在读取设置</p></div>
        <form v-else-if="form" id="application-settings-form" @submit.prevent="saveSettings">
          <template v-if="activeTab === 'upload'">
            <header class="settings-panel-heading">
              <span class="settings-icon"><svg viewBox="0 0 24 24"><path d="M12 16V4m0 0L7 9m5-5 5 5M5 14v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4"/></svg></span>
              <div><h2>上传设置</h2><p>这些限制会同时应用于界面和上传接口。</p></div>
            </header>

            <div class="settings-card">
              <div class="settings-card-title"><div><h3>上传限制</h3><p>控制单个请求可接收的数据规模。</p></div></div>
              <div class="settings-form-grid">
                <label class="field"><span>单张图片最大体积</span><div class="unit-input"><input v-model.number="form.upload.maxImageSizeMB" type="number" min="1" max="1024" required /><span>MB</span></div><small>可设置为 1–1024 MB</small></label>
                <label class="field"><span>单次上传最大图片数量</span><div class="unit-input"><input v-model.number="form.upload.maxImagesPerUpload" type="number" min="1" max="500" required /><span>张</span></div><small>可设置为 1–500 张</small></label>
              </div>
            </div>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>转换图片格式</h3><p>上传后统一编码为指定格式；动画只保留第一帧。</p></div>
                <label class="toggle-control"><input v-model="form.upload.convertImages" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.upload.convertImages ? '已开启' : '已关闭' }}</em></label>
              </div>
              <div v-if="form.upload.convertImages" class="dependent-settings">
                <div class="settings-form-grid">
                  <div class="field"><span>目标图片格式</span><BaseSelect v-model="form.upload.targetImageFormat" :options="targetFormatOptions" aria-label="目标图片格式" /><small>PNG 使用无损压缩，不需要质量参数</small></div>
                  <label v-if="supportsQuality" class="field"><span>压缩质量</span><div class="quality-input"><input v-model.number="form.upload.compressionQuality" type="range" min="1" max="100" /><input v-model.number="form.upload.compressionQuality" type="number" min="1" max="100" required /></div><small>1 体积更小，100 质量更高</small></label>
                </div>
              </div>
            </div>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>重命名图片</h3><p>为存储文件生成不暴露原始名称的唯一标识。</p></div>
                <label class="toggle-control"><input v-model="form.upload.renameImages" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.upload.renameImages ? '已开启' : '已关闭' }}</em></label>
              </div>
              <div v-if="form.upload.renameImages" class="dependent-settings">
                <div class="settings-form-grid">
                  <div class="field"><span>重命名方法</span><BaseSelect v-model="form.upload.renameMethod" :options="renameMethodOptions" aria-label="重命名方法" /><small>UUIDv5 对相同内容生成稳定标识；重复上传时仍会避免名称冲突</small></div>
                  <div class="inline-toggle-field"><div><strong>去除 UUID 连字符</strong><small>例如将 36 位 UUID 缩短为 32 个字符</small></div><label class="toggle-control icon-only"><input v-model="form.upload.stripUUIDHyphens" type="checkbox" /><span aria-hidden="true"></span></label></div>
                </div>
              </div>
              <p v-else class="settings-footnote">关闭后将使用安全化的原文件名；同名文件会自动添加数字后缀。</p>
            </div>
          </template>

          <template v-else-if="activeTab === 'security'">
            <header class="settings-panel-heading">
              <span class="settings-icon"><svg viewBox="0 0 24 24"><path d="M12 3 5 6v5c0 4.7 2.9 8.2 7 10 4.1-1.8 7-5.3 7-10V6l-7-3Z"/><path d="m9 12 2 2 4-4"/></svg></span>
              <div><h2>安全设置</h2><p>保护登录入口，控制公开图片跨域访问，并在可信代理后恢复真实来源 IP。</p></div>
            </header>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>限制登录失败次数</h3><p>按来源 IP 统计最近 15 分钟内的失败登录。</p></div>
                <label class="toggle-control"><input v-model="form.security.limitLoginFailures" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.security.limitLoginFailures ? '已开启' : '已关闭' }}</em></label>
              </div>
              <div v-if="form.security.limitLoginFailures" class="dependent-settings narrow-setting">
                <label class="field"><span>最大登录失败次数</span><div class="unit-input"><input v-model.number="form.security.maxLoginFailures" type="number" min="1" max="100" required /><span>次</span></div><small>达到上限后，该 IP 将被临时限制 15 分钟</small></label>
              </div>
            </div>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>公开图片 CORS</h3><p>允许其他网站通过 JavaScript 请求并缓存公开图片。</p></div>
                <label class="toggle-control"><input v-model="form.security.enablePublicCORS" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.security.enablePublicCORS ? '已开启' : '已关闭' }}</em></label>
              </div>
              <div v-if="form.security.enablePublicCORS" class="dependent-settings narrow-setting">
                <label class="field">
                  <span>允许来源</span>
                  <input v-model.trim="form.security.corsAllowedOrigin" type="text" placeholder="*" required />
                  <small>填写 <code>*</code> 允许任意来源，或填写一个完整来源，例如 <code>https://cache.example.com</code>；不要包含路径。</small>
                </label>
                <div class="security-warning"><strong>只开放公开图片资源</strong><span>该设置仅作用于 <code>/random</code> 和公开的 <code>/i/...</code>，不会发送 Cookie、开放管理 API 或暴露私密图片。</span></div>
              </div>
            </div>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>反向代理模式</h3><p>从指定请求标头读取访问者的真实 IP。</p></div>
                <label class="toggle-control"><input v-model="form.security.reverseProxyMode" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.security.reverseProxyMode ? '已开启' : '已关闭' }}</em></label>
              </div>
              <div v-if="form.security.reverseProxyMode" class="dependent-settings narrow-setting">
                <div class="field"><span>真实 IP 标头</span><BaseSelect v-model="form.security.realIPHeader" :options="realIPHeaderOptions" aria-label="真实 IP 标头" /><small>X-Forwarded-For 使用列表中的第一个有效 IP</small></div>
                <div class="security-warning"><strong>仅在服务只接受可信代理流量时开启</strong><span>如果客户端可以绕过代理直连，它可以伪造这个标头。</span></div>
              </div>
            </div>
          </template>

          <template v-else>
            <header class="settings-panel-heading">
              <span class="settings-icon"><svg viewBox="0 0 24 24"><path d="M8 9 5 12l3 3m8-6 3 3-3 3M14 5l-4 14"/></svg></span>
              <div><h2>API 设置</h2><p>配置公开接口的参数兼容性与默认行为。</p></div>
            </header>

            <div class="settings-card">
              <div class="settings-card-title"><div><h3>随机图片</h3><p>控制 <code>/random</code> 同时指定多个相册时的默认匹配方式。</p></div></div>
              <div class="dependent-settings narrow-setting">
                <div class="field">
                  <span>多相册默认操作</span>
                  <BaseSelect v-model="form.api.randomImageAlbumMode" :options="albumModeOptions" aria-label="多相册默认操作" />
                  <small>省略 <code>mode</code> 时使用；并集匹配任一相册，交集要求图片属于全部相册。</small>
                </div>
              </div>
            </div>

            <div class="settings-card">
              <div class="settings-card-title">
                <div><h3>忽略无效参数</h3><p>兼容会自动追加额外查询参数的主题、插件或缓存服务。</p></div>
                <label class="toggle-control"><input v-model="form.api.randomImageIgnoreUnknownParameters" type="checkbox" /><span aria-hidden="true"></span><em>{{ form.api.randomImageIgnoreUnknownParameters ? '已开启' : '已关闭' }}</em></label>
              </div>
              <p class="settings-footnote">开启后，<code>/random</code> 会忽略 <code>albums</code> 和 <code>mode</code> 以外的参数，例如 <code>1</code> 或 <code>type=mobile</code>；关闭时会返回 HTTP 400。已定义参数自身无效时仍返回 400。</p>
            </div>
          </template>

          <p v-if="saved" class="inline-message success"><span>✓</span>设置已保存并立即生效</p>
        </form>
      </div>
    </div>
  </section>
</template>
