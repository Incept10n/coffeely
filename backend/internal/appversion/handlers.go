package appversion

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Incept10n/coffeely/backend/internal/httpx"
)

// Handlers — публичная проверка обновлений и регистрация сборок.
type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers { return &Handlers{store: store} }

// GET /app/version?platform=android&version_code=1874
//
// Эндпоинт публичный: приложение спрашивает его до входа, иначе гости со старой
// сборкой упрутся в сломанный логин и не увидят, что нужно обновиться.
// Ответ — политика для этой версии. Пустой реестр = «обновлений нет»: сервер
// никогда не блокирует приложение из-за своей пустоты.
func (h *Handlers) Version(w http.ResponseWriter, r *http.Request) {
	platform := NormalizePlatform(r.URL.Query().Get("platform"))
	if platform == "" {
		httpx.WriteError(w, ErrBadPlatform)
		return
	}
	clientCode, err := strconv.Atoi(r.URL.Query().Get("version_code"))
	if err != nil || clientCode <= 0 {
		httpx.WriteError(w, ErrBadVersionCode)
		return
	}

	policy := Policy{
		Platform:          platform,
		ClientVersionCode: clientCode,
	}

	latest, err := h.store.Latest(r.Context(), platform)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, err)
		return
	}
	if err == nil {
		policy.LatestVersionCode = latest.VersionCode
		policy.LatestVersionName = latest.VersionName
		policy.DownloadURL = latest.DownloadURL
		policy.SHA256 = latest.SHA256
		policy.SizeBytes = latest.SizeBytes
		policy.Changelog = latest.Changelog
	}

	minSupported, err := h.store.MinSupported(r.Context(), platform)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	policy.MinSupportedVersionCode = minSupported

	policy.UpdateRequired = clientCode < minSupported
	policy.UpdateAvailable = latest.VersionCode > clientCode

	httpx.WriteJSON(w, http.StatusOK, policy)
}

type publishRequest struct {
	Platform    string `json:"platform"`
	VersionCode int    `json:"version_code"`
	VersionName string `json:"version_name"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	Mandatory   bool   `json:"mandatory"`
	Changelog   string `json:"changelog"`
}

// POST /internal/admin/app-versions — регистрация сборки (X-Admin-Secret).
// Вызывает CI после загрузки APK; повторная публикация обновляет запись.
func (h *Handlers) Publish(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.ReadJSON[publishRequest](r)
	if err != nil {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	release, err := Validate(Release{
		Platform:    req.Platform,
		VersionCode: req.VersionCode,
		VersionName: req.VersionName,
		DownloadURL: req.DownloadURL,
		SHA256:      req.SHA256,
		SizeBytes:   req.SizeBytes,
		Mandatory:   req.Mandatory,
		Changelog:   req.Changelog,
	})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	saved, err := h.store.Upsert(r.Context(), release)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, saved)
}

type mandatoryRequest struct {
	Mandatory *bool `json:"mandatory"`
}

// PATCH /internal/admin/app-versions/{platform}/{version_code} — блокировка и
// разблокировка уже выпущенной сборки без пересборки APK (X-Admin-Secret).
//
// Это аварийный рычаг: нашли ломающий баг — ставим mandatory=true на сборку с
// исправлением, все старые версии видят экран «обновитесь». Ошиблись — снимаем.
func (h *Handlers) SetMandatory(w http.ResponseWriter, r *http.Request) {
	platform := NormalizePlatform(r.PathValue("platform"))
	if platform == "" {
		httpx.WriteError(w, ErrBadPlatform)
		return
	}
	versionCode, err := strconv.Atoi(r.PathValue("version_code"))
	if err != nil || versionCode <= 0 {
		httpx.WriteError(w, ErrBadVersionCode)
		return
	}
	req, err := httpx.ReadJSON[mandatoryRequest](r)
	if err != nil || req.Mandatory == nil {
		httpx.WriteError(w, httpx.ErrBadRequest)
		return
	}
	release, err := h.store.SetMandatory(r.Context(), platform, versionCode, *req.Mandatory)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, release)
}
