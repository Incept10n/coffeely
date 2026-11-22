// Package appversion — реестр сборок мобильного приложения и политика обновлений.
//
// Приложение при старте спрашивает GET /app/version: сервер сравнивает его
// version_code с реестром и отвечает, есть ли новая сборка и обязательна ли она.
// Флаг mandatory стоит у конкретной сборки, поэтому минимально допустимая версия —
// это максимальный version_code среди mandatory-сборок. Следствия:
//   - обычная новая версия (mandatory=false) — мягкое предложение обновиться;
//   - одна запись mandatory=true блокирует вход со всех версий ниже неё;
//   - снять блокировку можно той же записью (PATCH mandatory=false), не трогая APK.
//
// Ту же политику читает лендинг, если захочет показать «последняя версия».
package appversion

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

var (
	ErrBadPlatform     = httpx.NewError(400, "bad_platform", "platform должна быть android или ios")
	ErrBadVersionCode  = httpx.NewError(400, "bad_version_code", "version_code должен быть положительным целым")
	ErrBadDownloadURL  = httpx.NewError(400, "bad_download_url", "download_url должен быть http(s)-ссылкой")
	ErrBadSHA256       = httpx.NewError(400, "bad_sha256", "sha256 должен быть 64 hex-символа")
	ErrReleaseNotFound = httpx.NewError(404, "release_not_found", "сборка не найдена в реестре")
)

// Release — одна сборка приложения в реестре.
type Release struct {
	Platform    string    `json:"platform"`
	VersionCode int       `json:"version_code"`
	VersionName string    `json:"version_name"`
	DownloadURL string    `json:"download_url"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"size_bytes"`
	Mandatory   bool      `json:"mandatory"`
	Changelog   string    `json:"changelog"`
	ReleasedAt  time.Time `json:"released_at"`
}

// Policy — ответ клиенту на «что мне делать с этой версией».
//
// UpdateAvailable — на сервере есть сборка новее клиентской (можно обновиться).
// UpdateRequired — клиентская версия ниже минимально допустимой (войти нельзя).
type Policy struct {
	Platform                string `json:"platform"`
	ClientVersionCode       int    `json:"client_version_code"`
	UpdateAvailable         bool   `json:"update_available"`
	UpdateRequired          bool   `json:"update_required"`
	LatestVersionCode       int    `json:"latest_version_code"`
	LatestVersionName       string `json:"latest_version_name"`
	MinSupportedVersionCode int    `json:"min_supported_version_code"`
	DownloadURL             string `json:"download_url"`
	SHA256                  string `json:"sha256"`
	SizeBytes               int64  `json:"size_bytes"`
	Changelog               string `json:"changelog"`
}

// Store — SQL реестра сборок.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Latest — самая новая сборка платформы; pgx.ErrNoRows, если реестр пуст.
func (s *Store) Latest(ctx context.Context, platform string) (Release, error) {
	var r Release
	err := s.pool.QueryRow(ctx, `
		SELECT platform, version_code, version_name, download_url, sha256,
		       size_bytes, mandatory, changelog, created_at
		FROM app_versions
		WHERE platform = $1
		ORDER BY version_code DESC
		LIMIT 1`, platform).
		Scan(&r.Platform, &r.VersionCode, &r.VersionName, &r.DownloadURL, &r.SHA256,
			&r.SizeBytes, &r.Mandatory, &r.Changelog, &r.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, pgx.ErrNoRows
	}
	if err != nil {
		return Release{}, fmt.Errorf("appversion: latest: %w", err)
	}
	return r, nil
}

// MinSupported — максимальная mandatory-сборка платформы (0, если блокирующих нет).
func (s *Store) MinSupported(ctx context.Context, platform string) (int, error) {
	var code int
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_code) FILTER (WHERE mandatory), 0)
		FROM app_versions
		WHERE platform = $1`, platform).Scan(&code)
	if err != nil {
		return 0, fmt.Errorf("appversion: min supported: %w", err)
	}
	return code, nil
}

// Upsert — регистрация сборки (CI после загрузки APK) или обновление её полей.
// Повторная публикация той же версии перезаписывает поля, а не падает.
func (s *Store) Upsert(ctx context.Context, r Release) (Release, error) {
	var out Release
	err := s.pool.QueryRow(ctx, `
		INSERT INTO app_versions
			(platform, version_code, version_name, download_url, sha256, size_bytes, mandatory, changelog)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (platform, version_code) DO UPDATE SET
			version_name = EXCLUDED.version_name,
			download_url = EXCLUDED.download_url,
			sha256       = EXCLUDED.sha256,
			size_bytes   = EXCLUDED.size_bytes,
			mandatory    = EXCLUDED.mandatory,
			changelog    = EXCLUDED.changelog,
			updated_at   = now()
		RETURNING platform, version_code, version_name, download_url, sha256,
		          size_bytes, mandatory, changelog, created_at`,
		r.Platform, r.VersionCode, r.VersionName, r.DownloadURL, r.SHA256,
		r.SizeBytes, r.Mandatory, r.Changelog).
		Scan(&out.Platform, &out.VersionCode, &out.VersionName, &out.DownloadURL, &out.SHA256,
			&out.SizeBytes, &out.Mandatory, &out.Changelog, &out.ReleasedAt)
	if err != nil {
		return Release{}, fmt.Errorf("appversion: upsert: %w", err)
	}
	return out, nil
}

// SetMandatory — включить или снять блокировку у уже зарегистрированной сборки.
func (s *Store) SetMandatory(ctx context.Context, platform string, versionCode int, mandatory bool) (Release, error) {
	var out Release
	err := s.pool.QueryRow(ctx, `
		UPDATE app_versions
		SET mandatory = $3, updated_at = now()
		WHERE platform = $1 AND version_code = $2
		RETURNING platform, version_code, version_name, download_url, sha256,
		          size_bytes, mandatory, changelog, created_at`,
		platform, versionCode, mandatory).
		Scan(&out.Platform, &out.VersionCode, &out.VersionName, &out.DownloadURL, &out.SHA256,
			&out.SizeBytes, &out.Mandatory, &out.Changelog, &out.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, ErrReleaseNotFound
	}
	if err != nil {
		return Release{}, fmt.Errorf("appversion: set mandatory: %w", err)
	}
	return out, nil
}

// NormalizePlatform приводит платформу к каноническому виду (” — неизвестная).
func NormalizePlatform(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "android":
		return "android"
	case "ios":
		return "ios"
	default:
		return ""
	}
}

var sha256Re = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Validate проверяет и нормализует сборку перед записью в реестр.
func Validate(r Release) (Release, error) {
	platform := NormalizePlatform(r.Platform)
	if platform == "" {
		return r, ErrBadPlatform
	}
	if r.VersionCode <= 0 {
		return r, ErrBadVersionCode
	}
	u, err := url.Parse(strings.TrimSpace(r.DownloadURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return r, ErrBadDownloadURL
	}
	r.Platform = platform
	r.DownloadURL = u.String()
	r.VersionName = strings.TrimSpace(r.VersionName)
	r.Changelog = strings.TrimSpace(r.Changelog)
	r.SHA256 = strings.ToLower(strings.TrimSpace(r.SHA256))
	if r.SizeBytes < 0 {
		r.SizeBytes = 0
	}
	// sha256 необязателен (старые сборки могли не иметь его), но если указан —
	// строго 64 hex: клиент сверяет его с файлом и на мусоре обязан падать.
	if r.SHA256 != "" && !sha256Re.MatchString(r.SHA256) {
		return r, ErrBadSHA256
	}
	return r, nil
}
