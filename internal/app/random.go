package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

const maxRandomImageAlbums = 20

var errRandomAlbumNotFound = errors.New("部分相册不存在")

type randomImageCandidate struct {
	StorageName string
}

func parseRandomAlbumNames(raw string) ([]string, string) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 || len(parts) > maxRandomImageAlbums {
		return nil, "随机图片接口最多支持 20 个相册"
	}
	unique := make(map[string]struct{}, len(parts))
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" || len([]rune(name)) > 80 {
			return nil, "相册名称列表无效"
		}
		key := strings.ToLower(name)
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, "请至少指定一个相册"
	}
	return names, ""
}

func (a *App) resolveRandomAlbumIDs(names []string) ([]int64, error) {
	arguments := make([]any, len(names))
	for index, name := range names {
		arguments[index] = name
	}
	rows, err := a.db.Query(`SELECT id FROM albums WHERE name IN (`+placeholders(len(names))+`)`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0, len(names))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) != len(names) {
		return nil, errRandomAlbumNotFound
	}
	return ids, nil
}

func (a *App) selectRandomPublicImage(albumIDs []int64, mode string) (randomImageCandidate, error) {
	var candidate randomImageCandidate
	if len(albumIDs) == 0 {
		err := a.db.QueryRow(`SELECT storage_name FROM images
			WHERE is_public = 1 ORDER BY RANDOM() LIMIT 1`).Scan(&candidate.StorageName)
		return candidate, err
	}

	arguments := make([]any, len(albumIDs))
	for index, id := range albumIDs {
		arguments[index] = id
	}
	placeholderList := placeholders(len(albumIDs))
	if mode == "intersection" {
		arguments = append(arguments, len(albumIDs))
		err := a.db.QueryRow(`SELECT i.storage_name FROM images i
			WHERE i.is_public = 1 AND i.id IN (
				SELECT ia.image_id FROM image_albums ia
				WHERE ia.album_id IN (`+placeholderList+`)
				GROUP BY ia.image_id HAVING COUNT(DISTINCT ia.album_id) = ?
			)
			ORDER BY RANDOM() LIMIT 1`, arguments...).Scan(&candidate.StorageName)
		return candidate, err
	}
	err := a.db.QueryRow(`SELECT i.storage_name FROM images i
		WHERE i.is_public = 1 AND EXISTS (
			SELECT 1 FROM image_albums ia
			WHERE ia.image_id = i.id AND ia.album_id IN (`+placeholderList+`)
		)
		ORDER BY RANDOM() LIMIT 1`, arguments...).Scan(&candidate.StorageName)
	return candidate, err
}

func (a *App) handleRandomImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.applyPublicCORSHeaders(w)
	apiSettings := a.currentSettings().API
	query := r.URL.Query()
	for key := range query {
		if key != "albums" && key != "mode" && !apiSettings.RandomImageIgnoreUnknownParameters {
			writeError(w, http.StatusBadRequest, "随机图片查询参数无效")
			return
		}
	}
	albumsValues, hasAlbums := query["albums"]
	modeValues, hasMode := query["mode"]
	if len(albumsValues) > 1 || len(modeValues) > 1 {
		writeError(w, http.StatusBadRequest, "随机图片查询参数不能重复")
		return
	}
	if !hasAlbums {
		if hasMode {
			writeError(w, http.StatusBadRequest, "mode 仅在指定 albums 时有效")
			return
		}
		candidate, err := a.selectRandomPublicImage(nil, "")
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "没有可用的公开图片")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法选择随机图片")
			return
		}
		http.Redirect(w, r, imageURL(candidate.StorageName), http.StatusFound)
		return
	}

	names, message := parseRandomAlbumNames(albumsValues[0])
	if message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	mode := ""
	if hasMode {
		mode = strings.TrimSpace(modeValues[0])
	}
	if mode == "" {
		mode = apiSettings.RandomImageAlbumMode
	}
	if mode != "union" && mode != "intersection" {
		writeError(w, http.StatusBadRequest, "mode 必须为 union 或 intersection")
		return
	}
	albumIDs, err := a.resolveRandomAlbumIDs(names)
	if errors.Is(err, errRandomAlbumNotFound) {
		writeError(w, http.StatusNotFound, errRandomAlbumNotFound.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取相册")
		return
	}
	candidate, err := a.selectRandomPublicImage(albumIDs, mode)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "没有符合条件的公开图片")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法选择随机图片")
		return
	}
	http.Redirect(w, r, imageURL(candidate.StorageName), http.StatusFound)
}
