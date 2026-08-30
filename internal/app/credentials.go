package app

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeAdministratorUsername(value string) (string, error) {
	username := strings.TrimSpace(value)
	if username == "" || len([]rune(username)) > 64 {
		return "", errors.New("用户名长度应为 1–64 个字符")
	}
	if strings.IndexFunc(username, func(character rune) bool {
		return unicode.IsControl(character) || unicode.Is(unicode.Cf, character) || unicode.Is(unicode.Zl, character) || unicode.Is(unicode.Zp, character)
	}) >= 0 {
		return "", errors.New("用户名不能包含控制或不可见格式字符")
	}
	return username, nil
}

func validateAdministratorPassword(password []byte) error {
	if !utf8.Valid(password) {
		return errors.New("密码必须是有效的 UTF-8 文本")
	}
	if utf8.RuneCount(password) < 8 || len(password) > 72 {
		return errors.New("密码至少需要 8 个字符，且不能超过 72 个字节")
	}
	return nil
}
