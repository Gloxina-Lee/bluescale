package app

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type albumRecord struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	ImageCount int    `json:"imageCount"`
}

func initializeAlbumDatabase(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS albums (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS image_albums (
			image_id INTEGER NOT NULL REFERENCES images(id) ON DELETE CASCADE,
			album_id INTEGER NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			PRIMARY KEY (image_id, album_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_image_albums_album_id_image_id ON image_albums(album_id, image_id)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleListAlbums(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(`SELECT a.id, a.name, a.created_at, COUNT(ia.image_id)
		FROM albums a LEFT JOIN image_albums ia ON ia.album_id = a.id
		GROUP BY a.id ORDER BY a.created_at DESC, a.id DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册列表")
		return
	}
	defer rows.Close()
	albums := make([]albumRecord, 0)
	for rows.Next() {
		var album albumRecord
		if err := rows.Scan(&album.ID, &album.Name, &album.CreatedAt, &album.ImageCount); err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取相册信息")
			return
		}
		albums = append(albums, album)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册列表")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": albums, "total": len(albums)})
}

type createAlbumRequest struct {
	Name string `json:"name"`
}

func normalizeAlbumName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return "", "相册名称长度应为 1–80 个字符"
	}
	return name, ""
}

func (a *App) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	var request createAlbumRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	name, message := normalizeAlbumName(request.Name)
	if message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	result, err := a.db.Exec(`INSERT INTO albums (name) VALUES (?)`, name)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeError(w, http.StatusConflict, "已存在同名相册")
			return
		}
		writeError(w, http.StatusInternalServerError, "无法创建相册")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册信息")
		return
	}
	var album albumRecord
	if err := a.db.QueryRow(`SELECT id, name, created_at, 0 FROM albums WHERE id = ?`, id).
		Scan(&album.ID, &album.Name, &album.CreatedAt, &album.ImageCount); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册信息")
		return
	}
	writeJSON(w, http.StatusCreated, album)
}

type albumIDsRequest struct {
	IDs []int64 `json:"ids"`
}

func normalizeIDs(ids []int64, maximum int) ([]int64, error) {
	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	if len(unique) == 0 || len(unique) > maximum {
		return nil, fmt.Errorf("请选择 1–%d 项", maximum)
	}
	result := make([]int64, 0, len(unique))
	for id := range unique {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func anyArguments(ids []int64) []any {
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	return arguments
}

func (a *App) handleDeleteAlbums(w http.ResponseWriter, r *http.Request) {
	var request albumIDsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	ids, err := normalizeIDs(request.IDs, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择 1–200 个相册")
		return
	}
	result, err := a.db.Exec(`DELETE FROM albums WHERE id IN (`+placeholders(len(ids))+`)`, anyArguments(ids)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法删除相册")
		return
	}
	deleted, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

type mergeAlbumsRequest struct {
	IDs      []int64 `json:"ids"`
	TargetID int64   `json:"targetId"`
}

func (a *App) handleMergeAlbums(w http.ResponseWriter, r *http.Request) {
	var request mergeAlbumsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	ids, err := normalizeIDs(request.IDs, 200)
	if err != nil || len(ids) < 2 {
		writeError(w, http.StatusBadRequest, "请选择至少两个相册进行合并")
		return
	}
	targetSelected := false
	for _, id := range ids {
		if id == request.TargetID {
			targetSelected = true
			break
		}
	}
	if !targetSelected {
		writeError(w, http.StatusBadRequest, "保留的相册必须在选中相册中")
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法开始合并相册")
		return
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM albums WHERE id IN (`+placeholders(len(ids))+`)`, anyArguments(ids)...).Scan(&existing); err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册")
		return
	}
	if existing != len(ids) {
		writeError(w, http.StatusNotFound, "部分相册已不存在")
		return
	}
	arguments := []any{request.TargetID}
	arguments = append(arguments, anyArguments(ids)...)
	if _, err := tx.Exec(`INSERT OR IGNORE INTO image_albums (image_id, album_id)
		SELECT image_id, ? FROM image_albums WHERE album_id IN (`+placeholders(len(ids))+`)`, arguments...); err != nil {
		writeError(w, http.StatusInternalServerError, "无法合并相册图片")
		return
	}
	deleteIDs := make([]int64, 0, len(ids)-1)
	for _, id := range ids {
		if id != request.TargetID {
			deleteIDs = append(deleteIDs, id)
		}
	}
	if _, err := tx.Exec(`DELETE FROM albums WHERE id IN (`+placeholders(len(deleteIDs))+`)`, anyArguments(deleteIDs)...); err != nil {
		writeError(w, http.StatusInternalServerError, "无法删除已合并相册")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法完成相册合并")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targetId": request.TargetID, "merged": len(deleteIDs)})
}

type imageAlbumsRequest struct {
	ImageIDs []int64 `json:"imageIds"`
	AlbumIDs []int64 `json:"albumIds"`
}

func validateExistingIDs(tx *sql.Tx, table string, ids []int64) error {
	var count int
	err := tx.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id IN (`+placeholders(len(ids))+`)`, anyArguments(ids)...).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return sql.ErrNoRows
	}
	return nil
}

func addImagesToAlbums(tx *sql.Tx, imageIDs, albumIDs []int64) error {
	if len(imageIDs) == 0 || len(albumIDs) == 0 {
		return nil
	}
	if err := validateExistingIDs(tx, "images", imageIDs); err != nil {
		return err
	}
	if err := validateExistingIDs(tx, "albums", albumIDs); err != nil {
		return err
	}
	imageArguments := anyArguments(imageIDs)
	albumArguments := anyArguments(albumIDs)
	arguments := append(imageArguments, albumArguments...)
	_, err := tx.Exec(`INSERT OR IGNORE INTO image_albums (image_id, album_id)
		SELECT i.id, a.id FROM images i CROSS JOIN albums a
		WHERE i.id IN (`+placeholders(len(imageIDs))+`) AND a.id IN (`+placeholders(len(albumIDs))+`)`, arguments...)
	return err
}

func decodeImageAlbumsRequest(w http.ResponseWriter, r *http.Request) ([]int64, []int64, error) {
	var request imageAlbumsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		return nil, nil, err
	}
	imageIDs, err := normalizeIDs(request.ImageIDs, 200)
	if err != nil {
		return nil, nil, errors.New("请选择 1–200 张图片")
	}
	albumIDs, err := normalizeIDs(request.AlbumIDs, 200)
	if err != nil {
		return nil, nil, errors.New("请选择 1–200 个相册")
	}
	return imageIDs, albumIDs, nil
}

func (a *App) handleAddImagesToAlbums(w http.ResponseWriter, r *http.Request) {
	imageIDs, albumIDs, err := decodeImageAlbumsRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法开始更新图片相册")
		return
	}
	defer tx.Rollback()
	if err := addImagesToAlbums(tx, imageIDs, albumIDs); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "部分图片或相册已不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "无法添加图片到相册")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "无法完成图片相册更新")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleRemoveImagesFromAlbums(w http.ResponseWriter, r *http.Request) {
	imageIDs, albumIDs, err := decodeImageAlbumsRequest(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	arguments := append(anyArguments(imageIDs), anyArguments(albumIDs)...)
	_, err = a.db.Exec(`DELETE FROM image_albums WHERE image_id IN (`+placeholders(len(imageIDs))+`)
		AND album_id IN (`+placeholders(len(albumIDs))+`)`, arguments...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法从相册移除图片")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
