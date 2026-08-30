package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
)

const applicationSettingsKey = "application_settings"

type uploadSettings struct {
	MaxImageSizeMB     int    `json:"maxImageSizeMB"`
	MaxImagesPerUpload int    `json:"maxImagesPerUpload"`
	ConvertImages      bool   `json:"convertImages"`
	TargetImageFormat  string `json:"targetImageFormat"`
	CompressionQuality int    `json:"compressionQuality"`
	RenameImages       bool   `json:"renameImages"`
	RenameMethod       string `json:"renameMethod"`
	StripUUIDHyphens   bool   `json:"stripUUIDHyphens"`
}

type securitySettings struct {
	LimitLoginFailures bool   `json:"limitLoginFailures"`
	MaxLoginFailures   int    `json:"maxLoginFailures"`
	ReverseProxyMode   bool   `json:"reverseProxyMode"`
	RealIPHeader       string `json:"realIPHeader"`
}

type apiSettings struct {
	RandomImageAlbumMode string `json:"randomImageAlbumMode"`
}

type applicationSettings struct {
	Upload   uploadSettings   `json:"upload"`
	Security securitySettings `json:"security"`
	API      apiSettings      `json:"api"`
}

func defaultApplicationSettings() applicationSettings {
	return applicationSettings{
		Upload: uploadSettings{
			MaxImageSizeMB: 50, MaxImagesPerUpload: 50,
			TargetImageFormat: "webp", CompressionQuality: 82,
			RenameImages: false, RenameMethod: "uuid_v4", StripUUIDHyphens: true,
		},
		Security: securitySettings{
			LimitLoginFailures: true,
			MaxLoginFailures:   5,
			RealIPHeader:       "X-Forwarded-For",
		},
		API: apiSettings{RandomImageAlbumMode: "union"},
	}
}

func loadApplicationSettings(db *sql.DB) (applicationSettings, error) {
	settings := defaultApplicationSettings()
	var encoded string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, applicationSettingsKey).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return applicationSettings{}, err
	}
	if err := json.Unmarshal([]byte(encoded), &settings); err != nil {
		return applicationSettings{}, err
	}
	if message := validateApplicationSettings(settings); message != "" {
		return applicationSettings{}, errors.New(message)
	}
	return settings, nil
}

func validateApplicationSettings(settings applicationSettings) string {
	if settings.Upload.MaxImageSizeMB < 1 || settings.Upload.MaxImageSizeMB > 1024 {
		return "单张图片最大体积应为 1–1024 MB"
	}
	if settings.Upload.MaxImagesPerUpload < 1 || settings.Upload.MaxImagesPerUpload > 500 {
		return "单次上传最大图片数量应为 1–500"
	}
	switch settings.Upload.TargetImageFormat {
	case "jpeg", "png", "webp", "avif":
	default:
		return "目标图片格式无效"
	}
	if settings.Upload.CompressionQuality < 1 || settings.Upload.CompressionQuality > 100 {
		return "压缩质量应为 1–100"
	}
	switch settings.Upload.RenameMethod {
	case "uuid_v4", "uuid_v5":
	default:
		return "图片重命名方法无效"
	}
	if settings.Security.MaxLoginFailures < 1 || settings.Security.MaxLoginFailures > 100 {
		return "最大登录失败次数应为 1–100"
	}
	switch settings.Security.RealIPHeader {
	case "X-Real-IP", "X-Forwarded-For", "CF-Connecting-IP":
	default:
		return "真实 IP 标头无效"
	}
	switch settings.API.RandomImageAlbumMode {
	case "union", "intersection":
	default:
		return "随机图片多相册默认操作无效"
	}
	return ""
}

func persistApplicationSettings(db *sql.DB, settings applicationSettings) error {
	encoded, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, applicationSettingsKey, string(encoded))
	return err
}

func (a *App) currentSettings() applicationSettings {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.settings
}

func (a *App) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.currentSettings())
}

func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings applicationSettings
	if err := decodeJSON(w, r, &settings); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if message := validateApplicationSettings(settings); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	if err := persistApplicationSettings(a.db, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存设置")
		return
	}
	a.settingsMu.Lock()
	a.settings = settings
	a.settingsMu.Unlock()
	a.clearLoginFailures()
	writeJSON(w, http.StatusOK, settings)
}

func (s uploadSettings) maxImageBytes() int64 {
	return int64(s.MaxImageSizeMB) << 20
}

func (s uploadSettings) maxRequestBytes() int64 {
	perImage := s.maxImageBytes()
	count := int64(s.MaxImagesPerUpload)
	if count > math.MaxInt64/perImage {
		return math.MaxInt64
	}
	total := perImage * count
	overhead := (count + 8) << 20
	if total > math.MaxInt64-overhead {
		return math.MaxInt64
	}
	return total + overhead
}
