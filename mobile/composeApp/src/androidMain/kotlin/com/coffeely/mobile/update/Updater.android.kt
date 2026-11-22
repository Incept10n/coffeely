package com.coffeely.mobile.update

import android.app.Activity
import android.app.DownloadManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Environment
import android.provider.Settings
import androidx.core.content.FileProvider
import java.io.File
import java.lang.ref.WeakReference
import java.security.MessageDigest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

// Контекст приложения и текущая Activity: нужны службе загрузок, установщику
// и «Закрыть приложение». init — из MainActivity до первого кадра.
internal object AndroidAppContext {
    private var appContext: Context? = null
    private var currentActivity: WeakReference<Activity>? = null

    fun init(activity: Activity) {
        appContext = activity.applicationContext
        currentActivity = WeakReference(activity)
    }

    fun app(): Context? = appContext
    fun activity(): Activity? = currentActivity?.get()
}

private const val APK_FILE = "coffeely-update.apk"
private const val REQUEST_INSTALL_PERMISSION = 26

private fun apkFile(ctx: Context): File =
    File(ctx.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS), APK_FILE)

// Скачивание идёт через DownloadManager, а не своим HTTP-клиентом:
// прогресс виден в шторке, загрузка переживает уход с экрана, а файл лежит
// в приватном каталоге приложения — установщику отдаём его через FileProvider.
internal actual suspend fun downloadAndInstall(
    url: String,
    sha256: String,
    onProgress: (DownloadProgress) -> Unit,
): DownloadProgress {
    val ctx = AndroidAppContext.app()
        ?: return failed("Приложение не инициализировано — перезапустите его.")
    if (url.isBlank()) return failed("Сервер не отдал ссылку на новую версию.")

    onProgress(DownloadProgress(DownloadPhase.DOWNLOADING, 0))
    // Старый файл удаляем: DownloadManager отказывается писать поверх
    // существующего файла (ERROR_FILE_ALREADY_EXISTS) после неудачной попытки.
    runCatching { apkFile(ctx).delete() }

    val manager = ctx.getSystemService(Context.DOWNLOAD_SERVICE) as? DownloadManager
        ?: return failed("Служба загрузок недоступна на этом устройстве.")
    val request = DownloadManager.Request(Uri.parse(url))
        .setTitle("coffeely")
        .setDescription("Скачивается обновление приложения")
        .setMimeType("application/vnd.android.package-archive")
        .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
        .setDestinationInExternalFilesDir(ctx, Environment.DIRECTORY_DOWNLOADS, APK_FILE)
        .setAllowedOverMetered(true)
    val id = runCatching { manager.enqueue(request) }.getOrElse {
        return failed("Не удалось начать скачивание: ${it.message}")
    }

    val row = awaitDownload(manager, id, onProgress)
        ?: return failed("Скачивание не удалось. Попробуйте ещё раз или скачайте в браузере.")
    if (row.status != DownloadManager.STATUS_SUCCESSFUL) {
        manager.remove(id)
        return failed("Скачивание прервалось (код ${row.reason}). Попробуйте ещё раз или скачайте в браузере.")
    }

    onProgress(DownloadProgress(DownloadPhase.VERIFYING, 100))
    val file = localFile(ctx, row.localUri) ?: return failed("Файл обновления не найден после скачивания.")
    if (sha256.isNotBlank()) {
        val actual = withContext(Dispatchers.IO) { sha256Of(file) }
        if (!actual.equals(sha256, ignoreCase = true)) {
            withContext(Dispatchers.IO) { file.delete() }
            manager.remove(id)
            return failed("Файл обновления повреждён: контрольная сумма не совпала. Скачайте ещё раз.")
        }
    }

    val error = installFile(ctx, file)
    return when (error) {
        InstallResult.Opened -> DownloadProgress(DownloadPhase.READY, 100, "Файл скачан и проверен.")
        InstallResult.MissingFile -> failed("Файл обновления не найден после скачивания.")
        is InstallResult.Blocked -> DownloadProgress(DownloadPhase.READY, 100, error.message)
    }
}

internal actual fun installDownloadedApk(): InstallResult {
    val ctx = AndroidAppContext.app() ?: return InstallResult.Blocked("Приложение не инициализировано — перезапустите его.")
    return installFile(ctx, apkFile(ctx))
}

internal actual fun openInBrowser(url: String) {
    if (url.isBlank()) return
    val ctx = AndroidAppContext.app() ?: return
    runCatching {
        ctx.startActivity(
            Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
        )
    }
}

internal actual fun closeApp() {
    AndroidAppContext.activity()?.finishAffinity()
}

private data class DownloadRow(
    val status: Int,
    val total: Long,
    val done: Long,
    val reason: Int,
    val localUri: String?,
)

// Опрос службы загрузок: прогресс обновляется каждые 400 мс, пока файл качается.
private suspend fun awaitDownload(
    manager: DownloadManager,
    id: Long,
    onProgress: (DownloadProgress) -> Unit,
): DownloadRow? {
    while (true) {
        delay(400)
        val row = query(manager, id) ?: return null
        when (row.status) {
            DownloadManager.STATUS_PENDING, DownloadManager.STATUS_RUNNING -> {
                val percent = if (row.total > 0) ((row.done * 100) / row.total).toInt() else -1
                onProgress(DownloadProgress(DownloadPhase.DOWNLOADING, percent))
            }

            else -> return row
        }
    }
}

private fun query(manager: DownloadManager, id: Long): DownloadRow? {
    val cursor = runCatching { manager.query(DownloadManager.Query().setFilterById(id)) }.getOrNull()
        ?: return null
    cursor.use { c ->
        if (!c.moveToFirst()) return null
        fun column(name: String) = c.getColumnIndexOrThrow(name)
        return DownloadRow(
            status = c.getInt(column(DownloadManager.COLUMN_STATUS)),
            total = c.getLong(column(DownloadManager.COLUMN_TOTAL_SIZE_BYTES)),
            done = c.getLong(column(DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR)),
            reason = c.getInt(column(DownloadManager.COLUMN_REASON)),
            localUri = c.getString(column(DownloadManager.COLUMN_LOCAL_URI)),
        )
    }
}

// DownloadManager отдаёт file:// URI; путь — в приватном каталоге приложения,
// поэтому читаем файл напрямую, без разрешений на внешнее хранилище.
private fun localFile(ctx: Context, localUri: String?): File? {
    val fromUri = localUri?.let { runCatching { File(Uri.parse(it).path ?: return@runCatching null) }.getOrNull() }
        ?.takeIf { it.exists() }
    if (fromUri != null) return fromUri
    return apkFile(ctx).takeIf { it.exists() }
}

private fun installFile(ctx: Context, file: File): InstallResult {
    if (!file.exists()) return InstallResult.MissingFile
    // Android 8+ (наш minSdk): установка вне Google Play требует разового
    // разрешения. Если его нет — открываем нужный экран настроек и просим
    // повторить: файл уже скачан, заново качать не нужно.
    if (!ctx.packageManager.canRequestPackageInstalls()) {
        runCatching {
            ctx.startActivity(
                Intent(
                    Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                    Uri.parse("package:${ctx.packageName}"),
                ).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
        }
        return InstallResult.Blocked(
            "Разрешите coffeely устанавливать приложения — это стандартный вопрос " +
                "Android при установке не из Google Play. Затем вернитесь и нажмите «Установить».",
        )
    }
    val uri = runCatching {
        FileProvider.getUriForFile(ctx, "${ctx.packageName}.updates", file)
    }.getOrElse { return InstallResult.Blocked("Не удалось подготовить файл к установке: ${it.message}") }

    val intent = Intent(Intent.ACTION_VIEW)
        .setDataAndType(uri, "application/vnd.android.package-archive")
        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    return runCatching { ctx.startActivity(intent); InstallResult.Opened }
        .getOrElse { InstallResult.Blocked("Установщик не открылся: ${it.message}. Попробуйте «Скачать в браузере».") }
}

private fun sha256Of(file: File): String {
    val digest = MessageDigest.getInstance("SHA-256")
    file.inputStream().use { input ->
        val buffer = ByteArray(64 * 1024)
        while (true) {
            val read = input.read(buffer)
            if (read <= 0) break
            digest.update(buffer, 0, read)
        }
    }
    return digest.digest().joinToString("") { "%02x".format(it) }
}

private fun failed(message: String) = DownloadProgress(DownloadPhase.FAILED, 0, message)
