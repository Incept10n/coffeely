package com.coffeely.mobile.update

// Состояние скачивания обновления для UI. Платформенная часть (Android —
// в update/Updater.android.kt) сообщает прогресс через колбэк.
enum class DownloadPhase { IDLE, DOWNLOADING, VERIFYING, READY, FAILED }

data class DownloadProgress(
    val phase: DownloadPhase = DownloadPhase.IDLE,
    /** 0..100; -1 — размер ещё неизвестен (прогресс-бар без процентов). */
    val percent: Int = 0,
    /** Текст проблемы или подсказка «что нажать» после загрузки. */
    val message: String = "",
)

// Скачать APK, сверить SHA-256 с серверным и открыть системный установщик.
// Возвращает финальное состояние: READY — файл готов (установщик уже открыт,
// его могли закрыть — тогда «Установить» ставит уже скачанный файл заново).
internal expect suspend fun downloadAndInstall(
    url: String,
    sha256: String,
    onProgress: (DownloadProgress) -> Unit,
): DownloadProgress

// Результат попытки открыть установщик.
internal sealed interface InstallResult {
    // Установщик открыт — дальше пользователь нажимает «Обновить/Установить».
    data object Opened : InstallResult

    // Файла нет (удалён/не докачался) — лечится только повторным скачиванием.
    data object MissingFile : InstallResult

    // Файл на месте, но установка не началась: нет разрешения, установщик
    // не открылся. Пользователь может повторить «Установить» без загрузки.
    data class Blocked(val message: String) : InstallResult
}

// Открыть установщик для ранее скачанного и проверенного файла.
internal expect fun installDownloadedApk(): InstallResult


// Запасной путь, если установщик упрямится: ссылка на APK откроется в браузере.
internal expect fun openInBrowser(url: String)

// Блокирующее обновление: внутрь попасть нельзя, поэтому приложение закрывается.
internal expect fun closeApp()
