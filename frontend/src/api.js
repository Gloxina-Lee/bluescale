export async function api(path, options = {}) {
  const headers = new Headers(options.headers || {})
  if (options.body && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(path, {
    ...options,
    headers,
    credentials: 'same-origin',
  })
  if (!response.ok) {
    let message = `请求失败 (${response.status})`
    try {
      const payload = await response.json()
      message = payload.error || message
    } catch {
      // Preserve the status-based fallback when the body is not JSON.
    }
    const error = new Error(message)
    error.status = response.status
    throw error
  }
  if (response.status === 204) return null
  return response.json()
}

export function uploadWithProgress(files, onProgress, albumIds = [], isPublic = false) {
  return new Promise((resolve, reject) => {
    const body = new FormData()
    files.forEach((file) => body.append('files', file))
    body.append('albumIds', JSON.stringify(albumIds))
    body.append('isPublic', String(isPublic))
    const request = new XMLHttpRequest()
    request.open('POST', '/api/images')
    request.withCredentials = true
    request.upload.addEventListener('progress', (event) => {
      if (event.lengthComputable) onProgress(Math.round((event.loaded / event.total) * 100))
    })
    request.addEventListener('load', () => {
      let payload = null
      try {
        payload = request.responseText ? JSON.parse(request.responseText) : null
      } catch {
        // Report a stable fallback below.
      }
      if (request.status >= 200 && request.status < 300) resolve(payload)
      else {
        const error = new Error(payload?.error || `上传失败 (${request.status})`)
        error.status = request.status
        reject(error)
      }
    })
    request.addEventListener('error', () => reject(new Error('网络连接失败')))
    request.send(body)
  })
}
