package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type imageRecord struct {
	ID           int64  `json:"id"`
	OriginalName string `json:"originalName"`
	StorageName  string `json:"storageName"`
	MimeType     string `json:"mimeType"`
	Size         int64  `json:"size"`
	CreatedAt    string `json:"createdAt"`
	URL          string `json:"url"`
}

var storageNamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|gif|webp|avif)$`)

var supportedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/avif": ".avif",
}

func (a *App) handleListImages(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(`SELECT id, original_name, storage_name, mime_type, size, created_at FROM images ORDER BY created_at DESC, id DESC LIMIT 1000`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片列表")
		return
	}
	defer rows.Close()
	images := make([]imageRecord, 0)
	for rows.Next() {
		var image imageRecord
		if err := rows.Scan(&image.ID, &image.OriginalName, &image.StorageName, &image.MimeType, &image.Size, &image.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取图片信息")
			return
		}
		image.URL = "/i/" + image.StorageName
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取图片列表")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": images, "total": len(images)})
}

func (a *App) handleUploadImages(w http.ResponseWriter, r *http.Request) {
	const maxRequestBytes int64 = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传内容无效或总大小超过 100 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "请选择至少一张图片")
		return
	}
	if len(files) > 50 {
		writeError(w, http.StatusBadRequest, "单次最多上传 50 张图片")
		return
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
	for _, header := range files {
		image, path, err := a.storeUpload(tx, header)
		if err != nil {
			cleanup()
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		createdPaths = append(createdPaths, path)
		uploaded = append(uploaded, image)
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		writeError(w, http.StatusInternalServerError, "无法保存上传记录")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"images": uploaded})
}

func (a *App) storeUpload(tx *sql.Tx, header *multipart.FileHeader) (imageRecord, string, error) {
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
		return imageRecord{}, "", fmt.Errorf("%s 不是支持的图片格式", filepath.Base(header.Filename))
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return imageRecord{}, "", errors.New("无法读取上传文件")
	}

	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return imageRecord{}, "", errors.New("无法生成图片地址")
	}
	storageName := hex.EncodeToString(token) + extension
	temporary, err := os.CreateTemp(a.imagesDir, ".upload-*")
	if err != nil {
		return imageRecord{}, "", errors.New("无法写入图片存储目录")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	written, copyErr := io.Copy(temporary, io.LimitReader(file, a.maxFileBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		return imageRecord{}, "", errors.New("无法保存图片")
	}
	if written > a.maxFileBytes {
		return imageRecord{}, "", fmt.Errorf("%s 超过 25 MB 限制", filepath.Base(header.Filename))
	}
	finalPath := filepath.Join(a.imagesDir, storageName)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return imageRecord{}, "", errors.New("无法保存图片")
	}

	cleanName := filepath.Base(strings.TrimSpace(header.Filename))
	if cleanName == "." || cleanName == "" {
		cleanName = "image" + extension
	}
	cleanRunes := []rune(cleanName)
	if len(cleanRunes) > 255 {
		cleanName = string(cleanRunes[:255])
	}
	result, err := tx.Exec(`INSERT INTO images (original_name, storage_name, mime_type, size) VALUES (?, ?, ?, ?)`, cleanName, storageName, mimeType, written)
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
	return imageRecord{ID: id, OriginalName: cleanName, StorageName: storageName, MimeType: mimeType, Size: written, CreatedAt: createdAt, URL: "/i/" + storageName}, finalPath, nil
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
	if !storageNamePattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	var mimeType string
	if err := a.db.QueryRow(`SELECT mime_type FROM images WHERE storage_name = ?`, name).Scan(&mimeType); err != nil {
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
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
	http.ServeContent(w, r, name, info.ModTime(), file)
}
