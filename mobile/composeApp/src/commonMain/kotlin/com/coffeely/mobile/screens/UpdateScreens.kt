package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.DialogProperties
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.AppVersionDto
import com.coffeely.mobile.api.currentVersionCode
import com.coffeely.mobile.api.currentVersionName
import com.coffeely.mobile.update.DownloadPhase
import com.coffeely.mobile.update.UpdateCenter
import com.coffeely.mobile.update.closeApp
import com.coffeely.mobile.update.openInBrowser
import kotlinx.coroutines.delay

/**
 * Обёртка всего приложения вокруг проверки обновлений.
 *
 * - Сервер пометил версию блокирующей (update_required) — вместо приложения
 *   непроходимый экран с установкой новой версии: старая сборка может ломать
 *   заказы, и «зайти и посмотреть» нельзя.
 * - Обновление обычное — мягкий попап при старте, «Позже» закрывает его на
 *   этот запуск; «О программе» умеет открыть попап заново.
 * - Проверка не удалась — приложение работает как обычно: сеть и сервер могут
 *   быть недоступны, а запирать пользователя из-за этого нельзя.
 */
@Composable
fun UpdateGate(api: ApiClient, content: @Composable () -> Unit) {
    LaunchedEffect(Unit) {
        UpdateCenter.refresh(api)
        // Приложение могут держать открытым сутками: раз в 15 минут смотрим,
        // не появилась ли блокирующая версия.
        while (true) {
            delay(15 * 60_000L)
            UpdateCenter.refresh(api)
        }
    }

    if (UpdateCenter.required) {
        RequiredUpdateScreen()
        return
    }

    Box(Modifier.fillMaxSize()) {
        content()
        if (UpdateCenter.shouldPrompt) SoftUpdateDialog()
    }
}

/** Мягкое обновление: попап при старте с возможностью отложить. */
@Composable
private fun SoftUpdateDialog() {
    val info = UpdateCenter.info ?: return
    val phase = UpdateCenter.download.phase
    val busy = phase == DownloadPhase.DOWNLOADING || phase == DownloadPhase.VERIFYING

    AlertDialog(
        onDismissRequest = { if (!busy) UpdateCenter.dismissSoft() },
        properties = DialogProperties(
            dismissOnBackPress = !busy,
            dismissOnClickOutside = !busy,
        ),
        title = { Text("Доступна новая версия") },
        text = {
            if (phase == DownloadPhase.IDLE) {
                UpdateSummary(info)
            } else {
                UpdateDownloadBody(info)
            }
        },
        confirmButton = {
            when {
                // Скачивание начинается по кнопке: трафик пользователя не
                // расходуем без согласия, а инструкция показывается уже в процессе.
                phase == DownloadPhase.IDLE ->
                    Button(onClick = { UpdateCenter.startDownload() }) { Text("Обновить") }

                // Загрузка не останавливается: она идёт в шторке уведомлений.
                busy -> TextButton(onClick = { UpdateCenter.dismissSoft() }) { Text("Скрыть") }

                // READY/FAILED — действия в теле диалога.
                else -> Unit
            }
        },
        dismissButton = {
            if (!busy) TextButton(onClick = { UpdateCenter.dismissSoft() }) { Text("Позже") }
        },
    )
}

/** Блокирующее обновление: экран вместо приложения, закрыть можно только приложение. */
@Composable
private fun RequiredUpdateScreen() {
    val info = UpdateCenter.info
    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .statusBarsPadding()
                .navigationBarsPadding()
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                "Нужно обновить приложение",
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
            )
            Text(
                "Версия $currentVersionName больше не поддерживается. В ней есть ошибки, " +
                    "из-за которых заказы и время готовности могут показываться неверно, " +
                    "поэтому вход временно закрыт.",
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                "Обновление займёт минуту и сохранит вход, заказы и настройки.",
                style = MaterialTheme.typography.bodyMedium,
            )

            if (info == null || info.download_url.isBlank()) {
                Text(
                    "Сервер не отдал ссылку на новую версию. Скачайте её на сайте coffeely.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            } else {
                // Никаких «Позже»: загрузка начинается сразу, чтобы минимум
                // действий был «дождаться и нажать Установить».
                LaunchedEffect(info.latest_version_code) { UpdateCenter.startDownload() }
                UpdateDownloadBody(info)
            }

            TextButton(onClick = { closeApp() }) { Text("Закрыть приложение") }

            Text(
                "Версия $currentVersionName · сборка $currentVersionCode",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun UpdateSummary(info: AppVersionDto) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Установлена версия $currentVersionName.")
        Text("Новая версия — ${info.latest_version_name.ifBlank { "сборка ${info.latest_version_code}" }}.")
        Text(
            "Обновление не обязательное: можно продолжить без него. " +
                "Но в старой версии могут быть уже исправленные ошибки.",
            style = MaterialTheme.typography.bodySmall,
        )
        if (info.changelog.isNotBlank()) {
            Text("Что нового: ${info.changelog}", style = MaterialTheme.typography.bodySmall)
        }
        if (info.size_bytes > 0) {
            Text("Размер файла — ${megabytes(info.size_bytes)}.", style = MaterialTheme.typography.bodySmall)
        }
    }
}

/**
 * Панель скачивания: прогресс и короткая инструкция, что нажимать.
 * Приложение ставится напрямую, не из Google Play, поэтому Android задаёт
 * лишние вопросы — их надо заранее объяснить, иначе установку бросят.
 */
@Composable
private fun UpdateDownloadBody(info: AppVersionDto) {
    val progress = UpdateCenter.download

    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        when (progress.phase) {
            DownloadPhase.IDLE -> {
                Button(onClick = { UpdateCenter.startDownload() }, modifier = Modifier.fillMaxWidth()) {
                    Text("Скачать обновление")
                }
            }

            DownloadPhase.DOWNLOADING -> {
                Text(
                    if (progress.percent >= 0) "Скачиваем обновление… ${progress.percent}%"
                    else "Скачиваем обновление…",
                    fontWeight = FontWeight.SemiBold,
                )
                if (progress.percent >= 0) {
                    LinearProgressIndicator(
                        progress = { progress.percent / 100f },
                        modifier = Modifier.fillMaxWidth(),
                    )
                } else {
                    LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                }
            }

            DownloadPhase.VERIFYING -> {
                Text("Проверяем файл…", fontWeight = FontWeight.SemiBold)
                LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
            }

            DownloadPhase.READY -> {
                Text(
                    progress.message.ifBlank { "Файл скачан и проверен." },
                    fontWeight = FontWeight.SemiBold,
                )
            }

            DownloadPhase.FAILED -> {
                Text(
                    progress.message.ifBlank { "Не удалось скачать обновление." },
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
                Button(onClick = { UpdateCenter.startDownload() }, modifier = Modifier.fillMaxWidth()) {
                    Text("Повторить")
                }
            }
        }

        UpdateInstructions(phase = progress.phase, sha256 = info.sha256)

        if (progress.phase == DownloadPhase.READY) {
            Button(onClick = { UpdateCenter.installDownloaded() }, modifier = Modifier.fillMaxWidth()) {
                Text("Установить")
            }
        }
        if (progress.phase == DownloadPhase.FAILED || progress.phase == DownloadPhase.READY) {
            TextButton(onClick = { openInBrowser(info.download_url) }) {
                Text("Скачать в браузере")
            }
        }
    }
}

@Composable
private fun UpdateInstructions(phase: DownloadPhase, sha256: String) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        when (phase) {
            DownloadPhase.READY -> {
                Text(
                    "Если установщик закрылся — нажмите «Установить»: файл уже скачан " +
                        "и проверен, заново качать не нужно.",
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            DownloadPhase.FAILED -> {
                Text(
                    "Можно повторить скачивание или открыть ссылку в браузере — " +
                        "там тот же файл.",
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            else -> {
                Text("Что будет дальше:", style = MaterialTheme.typography.labelLarge)
                Instruction("1", "Дождитесь конца загрузки — прогресс также виден в шторке уведомлений.")
                Instruction("2", "Откроется установщик Android: нажмите «Обновить» (или «Установить»).")
                Instruction(
                    "3",
                    "Android спросит разрешение на установку из этого источника — разрешите. " +
                        "Это стандартный вопрос при обновлении напрямую, не из Google Play.",
                )
                Instruction(
                    "4",
                    "Если Play Protect предложит проверку — согласитесь, проверка файла безопасна. " +
                        "Если появится «Всё равно установить» — подтвердите.",
                )
                Text(
                    "Приложение подписано нашим ключом, а контрольная сумма файла сверяется " +
                        "с сервером автоматически — подменённый файл установить не получится." +
                        if (sha256.isNotBlank()) "\nSHA-256: ${sha256.take(16)}…" else "",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun Instruction(number: String, text: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("$number.", style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.Bold)
        Text(text, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
    }
}

private fun megabytes(bytes: Long): String {
    if (bytes <= 0) return ""
    val mb = bytes.toDouble() / (1024 * 1024)
    return if (mb >= 1) "${mb.toInt()} МБ" else "${(mb * 1024).toInt()} КБ"
}
