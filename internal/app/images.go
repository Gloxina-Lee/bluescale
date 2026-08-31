package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/webp"
	"github.com/google/uuid"
)

const (
	maxDecodedPixels           int64 = 100_000_000
	publicImageCacheControl          = "public, max-age=0, must-revalidate"
	publicImageCDNCacheControl       = "public, max-age=86400"
	privateImageCacheControl         = "private, no-store"
)

type imageRecord struct {
	ID           int64  `json:"id"`
	OriginalName string `json:"originalName"`
	StorageName  string `json:"storageName"`
	MimeType     string `json:"mimeType"`
	Size         int64  `json:"size"`
	IsPublic     bool   `json:"isPublic"`
	CreatedAt    string `json:"createdAt"`
	URL          string `json:"url"`
}

var supportedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/avif": ".avif",
}

var imageFormatMIMEs = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"avif": "image/avif",
}

var imageUUIDNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://bluescale.local/images"))

func migrateImageStorage(db *sql.DB, imagesDir string) error {
	hasOwner, err := tableHasColumn(db, "images", "user_id")
	if err != nil || !hasOwner {
		return err
	}
	rows, err := db.Query(`SELECT user_id, storage_name FROM images`)
	if err != nil {
		return err
	}
	type legacyImage struct {
		userID int64
		name   string
	}
	images := make([]legacyImage, 0)
	for rows.Next() {
		var image legacyImage
		if err := rows.Scan(&image.userID, &image.name); err != nil {
			rows.Close()
			return err
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, image := range images {
		if !validStorageName(image.name) {
			return fmt.Errorf("invalid legacy image storage name %q", image.name)
		}
		oldPath := filepath.Join(imagesDir, strconv.FormatInt(image.userID, 10), image.name)
		newPath := filepath.Join(imagesDir, image.name)
		if _, err := os.Stat(newPath); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(oldPath, newPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			_ = os.Remove(filepath.Join(imagesDir, entry.Name()))
		}
	}
	return nil
}

func imageURL(name string) string {
	return "/i/" + url.PathEscape(name)
}

func validStorageName(name string) bool {
	if name == "" || len([]rune(name)) > 255 || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	_, ok := supportedImageTypes[mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))]
	return ok
}

func (a *App) handleListImages(w http.ResponseWriter, r *http.Request) {
	_, authenticated, authErr := a.authenticateRequest(r)
	if authErr != nil {
		writeError(w, http.StatusInternalServerError, "无法验证身份凭据")
		return
	}
	page, err := positiveQueryInteger(r, "page", 1, 1, 1_000_000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "页码无效")
		return
	}
	pageSize, err := positiveQueryInteger(r, "pageSize", 24, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "每页图片数量应为 1–200")
		return
	}
	conditions := make([]string, 0, 3)
	arguments := make([]any, 0, 3)
	if !authenticated {
		conditions = append(conditions, "i.is_public = 1")
	}
	visibility := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("visibility")))
	if visibility != "" && visibility != "all" {
		if visibility != "public" && visibility != "private" {
			writeError(w, http.StatusBadRequest, "图片可见范围筛选条件无效")
			return
		}
		if visibility == "public" {
			conditions = append(conditions, "i.is_public = 1")
		} else {
			conditions = append(conditions, "i.is_public = 0")
		}
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "" && format != "all" {
		mimeType, ok := imageFormatMIMEs[format]
		if !ok {
			writeError(w, http.StatusBadRequest, "图片格式筛选条件无效")
			return
		}
		conditions = append(conditions, "i.mime_type = ?")
		arguments = append(arguments, mimeType)
	}
	albumValue := strings.TrimSpace(r.URL.Query().Get("album"))
	if albumValue == "none" {
		conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM image_albums ia WHERE ia.image_id = i.id)")
	} else if albumValue != "" {
		albumID, parseErr := strconv.ParseInt(albumValue, 10, 64)
		if parseErr != nil || albumID <= 0 {
			writeError(w, http.StatusBadRequest, "相册筛选条件无效")
			return
		}
		conditions = append(conditions, "EXISTS (SELECT 1 FROM image_albums ia WHERE ia.image_id = i.id AND ia.album_id = ?)")
		arguments = append(arguments, albumID)
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM images i`+where, arguments...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片数量")
		return
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if page > totalPages {
			page = totalPages
		}
	} else {
		page = 1
	}
	queryArguments := append(append([]any(nil), arguments...), pageSize, (page-1)*pageSize)
	rows, err := a.db.Query(`SELECT i.id, i.original_name, i.storage_name, i.mime_type, i.size, i.is_public, i.created_at
		FROM images i`+where+` ORDER BY i.created_at DESC, i.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片列表")
		return
	}
	defer rows.Close()
	images := make([]imageRecord, 0)
	for rows.Next() {
		var image imageRecord
		if err := rows.Scan(&image.ID, &image.OriginalName, &image.StorageName, &image.MimeType, &image.Size, &image.IsPublic, &image.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取图片信息")
			return
		}
		image.URL = imageURL(image.StorageName)
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片列表")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"images": images, "total": total, "page": page, "pageSize": pageSize, "totalPages": totalPages,
	})
}

func positiveQueryInteger(r *http.Request, name string, fallback, minimum, maximum int) (int, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func (a *App) handleUploadImages(w http.ResponseWriter, r *http.Request) {
	settings := a.currentSettings().Upload
	r.Body = http.MaxBytesReader(w, r.Body, settings.maxRequestBytes())
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传内容无效或总大小超过当前限制")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "请选择至少一张图片")
		return
	}
	if len(files) > settings.MaxImagesPerUpload {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("单次最多上传 %d 张图片", settings.MaxImagesPerUpload))
		return
	}
	albumIDs, err := parseUploadAlbumIDs(r.FormValue("albumIds"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	isPublic := false
	if value := strings.TrimSpace(r.FormValue("isPublic")); value != "" {
		if value != "true" && value != "false" {
			writeError(w, http.StatusBadRequest, "图片公开属性无效")
			return
		}
		isPublic = value == "true"
	}

	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法开始上传")
		return
	}
	defer tx.Rollback()
	createdPaths := make([]string, 0, len(files))
	cleanup := func() {
		for _, path := range createdPaths {
			_ = os.Remove(path)
		}
	}

	uploaded := make([]imageRecord, 0, len(files))
	uploadedIDs := make([]int64, 0, len(files))
	for _, header := range files {
		image, path, err := a.storeUpload(tx, header, settings, isPublic)
		if err != nil {
			cleanup()
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		createdPaths = append(createdPaths, path)
		uploaded = append(uploaded, image)
		uploadedIDs = append(uploadedIDs, image.ID)
	}
	if len(albumIDs) > 0 {
		if err := addImagesToAlbums(tx, uploadedIDs, albumIDs); errors.Is(err, sql.ErrNoRows) {
			cleanup()
			writeError(w, http.StatusBadRequest, "选择的部分相册已不存在")
			return
		} else if err != nil {
			cleanup()
			writeError(w, http.StatusInternalServerError, "无法把图片加入相册")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		writeError(w, http.StatusInternalServerError, "无法保存上传记录")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"images": uploaded})
}

func parseUploadAlbumIDs(encoded string) ([]int64, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(encoded), &ids); err != nil {
		return nil, errors.New("上传相册选择无效")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	normalized, err := normalizeIDs(ids, 200)
	if err != nil {
		return nil, errors.New("单次最多选择 200 个相册")
	}
	return normalized, nil
}

func (a *App) storeUpload(tx *sql.Tx, header *multipart.FileHeader, settings uploadSettings, isPublic bool) (imageRecord, string, error) {
	displayName := cleanOriginalName(header.Filename, ".img")
	if header.Size > settings.maxImageBytes() {
		return imageRecord{}, "", fmt.Errorf("%s 超过 %d MB 限制", displayName, settings.MaxImageSizeMB)
	}
	file, err := header.Open()
	if err != nil {
		return imageRecord{}, "", errors.New("无法读取上传文件")
	}
	defer file.Close()

	buffer := make([]byte, 512)
	read, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return imageRecord{}, "", errors.New("图片内容为空或无法读取")
	}
	buffer = buffer[:read]
	mimeType := detectImageMIME(buffer)
	extension, ok := supportedImageTypes[mimeType]
	if !ok {
		return imageRecord{}, "", fmt.Errorf("%s 不是支持的图片格式", displayName)
	}
	displayName = cleanOriginalName(header.Filename, extension)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return imageRecord{}, "", errors.New("无法读取上传文件")
	}

	temporary, err := os.CreateTemp(a.imagesDir, ".upload-*")
	if err != nil {
		return imageRecord{}, "", errors.New("无法写入图片存储目录")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if settings.ConvertImages {
		convertedMime, convertedExtension, err := convertImage(temporary, file, mimeType, settings.TargetImageFormat, settings.CompressionQuality)
		if err != nil {
			temporary.Close()
			return imageRecord{}, "", fmt.Errorf("无法转换 %s：%v", displayName, err)
		}
		mimeType = convertedMime
		extension = convertedExtension
		displayName = replaceImageExtension(displayName, extension)
	} else {
		written, err := io.Copy(temporary, io.LimitReader(file, settings.maxImageBytes()+1))
		if err != nil {
			temporary.Close()
			return imageRecord{}, "", errors.New("无法保存图片")
		}
		if written > settings.maxImageBytes() {
			temporary.Close()
			return imageRecord{}, "", fmt.Errorf("%s 超过 %d MB 限制", displayName, settings.MaxImageSizeMB)
		}
	}
	if err := temporary.Close(); err != nil {
		return imageRecord{}, "", errors.New("无法保存图片")
	}
	info, err := os.Stat(temporaryPath)
	if err != nil {
		return imageRecord{}, "", errors.New("无法保存图片")
	}
	if info.Size() > settings.maxImageBytes() {
		return imageRecord{}, "", fmt.Errorf("%s 转换后的文件超过 %d MB 限制", displayName, settings.MaxImageSizeMB)
	}

	storageName, err := a.chooseStorageName(tx, temporaryPath, displayName, extension, settings)
	if err != nil {
		return imageRecord{}, "", err
	}
	finalPath := filepath.Join(a.imagesDir, storageName)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return imageRecord{}, "", errors.New("无法保存图片")
	}

	result, err := tx.Exec(`INSERT INTO images (original_name, storage_name, mime_type, size, is_public) VALUES (?, ?, ?, ?, ?)`, displayName, storageName, mimeType, info.Size(), isPublic)
	if err != nil {
		_ = os.Remove(finalPath)
		return imageRecord{}, "", errors.New("无法保存图片记录")
	}
	id, err := result.LastInsertId()
	if err != nil {
		_ = os.Remove(finalPath)
		return imageRecord{}, "", errors.New("无法读取图片记录")
	}
	var createdAt string
	if err := tx.QueryRow(`SELECT created_at FROM images WHERE id = ?`, id).Scan(&createdAt); err != nil {
		_ = os.Remove(finalPath)
		return imageRecord{}, "", errors.New("无法读取图片记录")
	}
	return imageRecord{ID: id, OriginalName: displayName, StorageName: storageName, MimeType: mimeType, Size: info.Size(), IsPublic: isPublic, CreatedAt: createdAt, URL: imageURL(storageName)}, finalPath, nil
}

func cleanOriginalName(name, fallbackExtension string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		name = "image" + fallbackExtension
	}
	runes := []rune(name)
	if len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

func replaceImageExtension(name, extension string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" {
		stem = "image"
	}
	return cleanOriginalName(stem+extension, extension)
}

func convertImage(destination io.Writer, source io.ReadSeeker, sourceMIME, targetFormat string, quality int) (string, string, error) {
	config, err := decodeImageConfig(source, sourceMIME)
	if err != nil {
		return "", "", errors.New("图片内容损坏或无法解码")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxDecodedPixels/int64(config.Height) {
		return "", "", errors.New("图片像素尺寸过大")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	decoded, err := decodeImage(source, sourceMIME)
	if err != nil {
		return "", "", errors.New("图片内容损坏或无法解码")
	}

	switch targetFormat {
	case "jpeg":
		bounds := decoded.Bounds()
		flattened := image.NewRGBA(bounds)
		draw.Draw(flattened, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(flattened, bounds, decoded, bounds.Min, draw.Over)
		return "image/jpeg", ".jpg", jpeg.Encode(destination, flattened, &jpeg.Options{Quality: quality})
	case "png":
		return "image/png", ".png", (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(destination, decoded)
	case "webp":
		return "image/webp", ".webp", webp.Encode(destination, decoded, webp.Options{Quality: quality, Method: 4})
	case "avif":
		return "image/avif", ".avif", avif.Encode(destination, decoded, avif.Options{Quality: quality, QualityAlpha: quality, Speed: 8})
	default:
		return "", "", errors.New("目标图片格式无效")
	}
}

func decodeImageConfig(source io.ReadSeeker, mimeType string) (image.Config, error) {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return image.Config{}, err
	}
	switch mimeType {
	case "image/jpeg":
		return jpeg.DecodeConfig(source)
	case "image/png":
		return png.DecodeConfig(source)
	case "image/gif":
		return gif.DecodeConfig(source)
	case "image/webp":
		return webp.DecodeConfig(source)
	case "image/avif":
		return avif.DecodeConfig(source)
	default:
		return image.Config{}, errors.New("不支持的图片格式")
	}
}

func decodeImage(source io.Reader, mimeType string) (image.Image, error) {
	switch mimeType {
	case "image/jpeg":
		return jpeg.Decode(source)
	case "image/png":
		return png.Decode(source)
	case "image/gif":
		return gif.Decode(source)
	case "image/webp":
		return webp.Decode(source, webp.Options{AutoRotate: true})
	case "image/avif":
		return avif.Decode(source, avif.Options{AutoRotate: true})
	default:
		return nil, errors.New("不支持的图片格式")
	}
}

func (a *App) chooseStorageName(tx *sql.Tx, temporaryPath, displayName, extension string, settings uploadSettings) (string, error) {
	var contentDigest [sha256.Size]byte
	if settings.RenameImages && settings.RenameMethod == "uuid_v5" {
		file, err := os.Open(temporaryPath)
		if err != nil {
			return "", errors.New("无法读取转换后的图片")
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return "", errors.New("无法读取转换后的图片")
		}
		copy(contentDigest[:], digest.Sum(nil))
	}

	base := sanitizeStorageStem(strings.TrimSuffix(displayName, filepath.Ext(displayName)))
	for attempt := 0; attempt < 10_000; attempt++ {
		var candidate string
		if !settings.RenameImages {
			candidate = base
			if attempt > 0 {
				candidate += "-" + strconv.Itoa(attempt+1)
			}
		} else {
			var identifier uuid.UUID
			if settings.RenameMethod == "uuid_v5" {
				seed := append([]byte(nil), contentDigest[:]...)
				seed = strconv.AppendInt(seed, int64(attempt), 10)
				identifier = uuid.NewSHA1(imageUUIDNamespace, seed)
			} else {
				identifier = uuid.New()
			}
			candidate = identifier.String()
			if settings.StripUUIDHyphens {
				candidate = strings.ReplaceAll(candidate, "-", "")
			}
		}
		candidate += extension
		available, err := a.storageNameAvailable(tx, candidate)
		if err != nil {
			return "", errors.New("无法检查图片名称")
		}
		if available {
			return candidate, nil
		}
	}
	return "", errors.New("无法生成唯一的图片名称")
}

func sanitizeStorageStem(stem string) string {
	stem = strings.TrimSpace(stem)
	var builder strings.Builder
	for _, character := range stem {
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\|?*`, character) {
			builder.WriteRune('_')
		} else {
			builder.WriteRune(character)
		}
	}
	stem = strings.TrimRight(builder.String(), ". ")
	if stem == "" {
		stem = "image"
	}
	upper := strings.ToUpper(stem)
	reserved := upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL"
	for index := 1; index <= 9 && !reserved; index++ {
		reserved = upper == "COM"+strconv.Itoa(index) || upper == "LPT"+strconv.Itoa(index)
	}
	if reserved {
		stem = "_" + stem
	}
	runes := []rune(stem)
	if len(runes) > 200 {
		stem = string(runes[:200])
	}
	return stem
}

func (a *App) storageNameAvailable(tx *sql.Tx, name string) (bool, error) {
	if !validStorageName(name) {
		return false, nil
	}
	var exists int
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM images WHERE storage_name = ?)`, name).Scan(&exists); err != nil {
		return false, err
	}
	if exists != 0 {
		return false, nil
	}
	_, err := os.Stat(filepath.Join(a.imagesDir, name))
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func detectImageMIME(header []byte) string {
	if len(header) >= 12 && string(header[4:8]) == "ftyp" {
		brand := string(header[8:12])
		if brand == "avif" || brand == "avis" {
			return "image/avif"
		}
	}
	return http.DetectContentType(header)
}

type deleteImagesRequest struct {
	IDs []int64 `json:"ids"`
}

type updateImageVisibilityRequest struct {
	IDs      []int64 `json:"ids"`
	IsPublic bool    `json:"isPublic"`
}

func (a *App) handleUpdateImageVisibility(w http.ResponseWriter, r *http.Request) {
	var request updateImageVisibilityRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	ids, err := normalizeIDs(request.IDs, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择 1–200 张图片")
		return
	}
	result, err := a.db.Exec(`UPDATE images SET is_public = ? WHERE id IN (`+placeholders(len(ids))+`)`, append([]any{request.IsPublic}, anyArguments(ids)...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法更新图片公开属性")
		return
	}
	updated, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "isPublic": request.IsPublic})
}

type pendingDeletion struct {
	id       int64
	original string
	trash    string
}

func (a *App) handleDeleteImages(w http.ResponseWriter, r *http.Request) {
	var request deleteImagesRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	unique := make(map[int64]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	if len(unique) == 0 || len(unique) > 200 {
		writeError(w, http.StatusBadRequest, "请选择 1–200 张图片")
		return
	}
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}

	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法开始删除")
		return
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, storage_name FROM images WHERE id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取待删除图片")
		return
	}
	deletions := make([]pendingDeletion, 0, len(ids))
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "无法读取待删除图片")
			return
		}
		deletions = append(deletions, pendingDeletion{id: id, original: filepath.Join(a.imagesDir, name), trash: filepath.Join(a.imagesDir, ".trash-"+name)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "无法读取待删除图片")
		return
	}
	rows.Close()

	moved := make([]pendingDeletion, 0, len(deletions))
	for _, deletion := range deletions {
		if err := os.Rename(deletion.original, deletion.trash); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				for _, prior := range moved {
					_ = os.Rename(prior.trash, prior.original)
				}
				writeError(w, http.StatusInternalServerError, "无法删除图片文件")
				return
			}
			continue
		}
		moved = append(moved, deletion)
	}
	if _, err := tx.Exec(`DELETE FROM images WHERE id IN (`+placeholders+`)`, arguments...); err != nil {
		for _, deletion := range moved {
			_ = os.Rename(deletion.trash, deletion.original)
		}
		writeError(w, http.StatusInternalServerError, "无法删除图片记录")
		return
	}
	if err := tx.Commit(); err != nil {
		for _, deletion := range moved {
			_ = os.Rename(deletion.trash, deletion.original)
		}
		writeError(w, http.StatusInternalServerError, "无法完成删除")
		return
	}
	for _, deletion := range moved {
		_ = os.Remove(deletion.trash)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleServeImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validStorageName(name) {
		http.NotFound(w, r)
		return
	}
	var mimeType string
	var isPublic bool
	if err := a.db.QueryRow(`SELECT mime_type, is_public FROM images WHERE storage_name = ?`, name).Scan(&mimeType, &isPublic); err != nil {
		http.NotFound(w, r)
		return
	}
	if !isPublic {
		w.Header().Set("Cache-Control", privateImageCacheControl)
		_, authenticated, err := a.authenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法验证身份凭据")
			return
		}
		if !authenticated {
			http.NotFound(w, r)
			return
		}
	}
	cacheControl := privateImageCacheControl
	if isPublic {
		cacheControl = publicImageCacheControl
		w.Header().Set("CDN-Cache-Control", publicImageCDNCacheControl)
		a.applyPublicCORSHeaders(w)
	}
	a.serveStoredImage(w, r, name, mimeType, cacheControl)
}

func (a *App) serveStoredImage(w http.ResponseWriter, r *http.Request, name, mimeType, cacheControl string) {
	if !validStorageName(name) {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filepath.Join(a.imagesDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	http.ServeContent(w, r, name, info.ModTime(), file)
}
