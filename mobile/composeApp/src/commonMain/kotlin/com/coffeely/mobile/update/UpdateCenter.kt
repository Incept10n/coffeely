package com.coffeely.mobile.update

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.AppVersionDto
import com.coffeely.mobile.api.currentVersionCode
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

// Политика обновлений и состояние скачивания в памяти процесса. Живёт вне
// Compose-дерева: экран блокировки, попап на старте и «О программе» смотрят на
// одно состояние, а скачивание не обрывается, когда пользователь закрывает
// попап — загрузка продолжается в шторке уведомлений.
object UpdateCenter {
    private val json = Json { ignoreUnknownKeys = true }

    // Скачивание переживает закрытие попапа, поэтому своя область, а не
    // rememberCoroutineScope у диалога (тот отменяется вместе с композицией).
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    var info: AppVersionDto? by mutableStateOf(null)
        private set

    // Проверка не удалась: без сети приложение работает так, как умеет, и не
    // запирает пользователя. Последняя известная политика при этом остаётся.
    var checkFailed: Boolean by mutableStateOf(false)
        private set

    var checking: Boolean by mutableStateOf(false)
        private set

    // «Позже» на текущий запуск. Блокирующее обновление важнее: его «Позже» не
    // прячет.
    var softDismissed: Boolean by mutableStateOf(false)
        private set

    // Пользователь сам открыл окно обновления («О программе» → проверить).
    var promptRequested: Boolean by mutableStateOf(false)
        private set

    var download: DownloadProgress by mutableStateOf(DownloadProgress())
        private set

    val required: Boolean
        get() = info?.let { currentVersionCode < it.min_supported_version_code } == true

    val available: Boolean
        get() = info?.let { currentVersionCode < it.latest_version_code } == true

    val shouldPrompt: Boolean
        get() = available && (!softDismissed || promptRequested)

    suspend fun refresh(api: ApiClient) {
        checking = true
        val result = api.appVersion(currentVersionCode)
        checking = false
        result.fold(
            onSuccess = { dto ->
                checkFailed = false
                // Новая версия появилась уже после «Позже» — показываем снова.
                if (dto.latest_version_code != info?.latest_version_code) softDismissed = false
                info = dto
                UpdateStorage.save(json.encodeToString(dto))
            },
            onFailure = {
                checkFailed = true
                // Ответ мог не дойти (сеть, 5xx). Если в памяти ничего нет —
                // берём последнюю сохранённую политику: блокировку не должен
                // снимать перезапуск приложения без интернета.
                if (info == null) {
                    info = UpdateStorage.load()?.let { raw ->
                        runCatching { json.decodeFromString<AppVersionDto>(raw) }.getOrNull()
                    }
                }
            },
        )
    }

    // Начать скачивание текущей новой сборки. Повторный вызов во время
    // загрузки — no-op: второй файл поверх первого DownloadManager не запишет.
    fun startDownload() {
        val dto = info ?: return
        val phase = download.phase
        if (phase == DownloadPhase.DOWNLOADING || phase == DownloadPhase.VERIFYING) return
        if (dto.download_url.isBlank()) {
            download = DownloadProgress(DownloadPhase.FAILED, 0, "Сервер не отдал ссылку на новую версию.")
            return
        }
        download = DownloadProgress(DownloadPhase.DOWNLOADING, 0)
        scope.launch {
            download = downloadAndInstall(dto.download_url, dto.sha256) { progress ->
                download = progress
            }
        }
    }

    // «Установить» для уже скачанного файла: пользователь мог закрыть установщик
    // или не выдать разрешение на установку — заново качать файл не нужно.
    fun installDownloaded() {
        download = when (val result = installDownloadedApk()) {
            InstallResult.Opened ->
                DownloadProgress(DownloadPhase.READY, 100, "Установщик открыт — нажмите «Обновить».")

            InstallResult.MissingFile ->
                DownloadProgress(DownloadPhase.FAILED, 0, "Файл обновления не найден. Скачайте его ещё раз.")

            is InstallResult.Blocked ->
                DownloadProgress(DownloadPhase.READY, 100, result.message)
        }
    }

    fun dismissSoft() {
        softDismissed = true
        promptRequested = false
    }

    fun requestPrompt() {
        promptRequested = true
    }
}

// Последняя политика на устройстве (SharedPreferences): нужна только для
// офлайн-старта, поэтому хранится сырым JSON и молча игнорируется при сбое.
internal expect object UpdateStorage {
    fun save(json: String)
    fun load(): String?
}
