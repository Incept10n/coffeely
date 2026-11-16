package com.coffeely.mobile.screens

// KDS-борда баристы (M9): заказы разложены по категориям статусов на одном экране,
// карточка светится цветом своей категории; при смене статуса карточка перемещается
// в другую категорию. Признак «новый · нет ответа» — это статус paid: первый ответ
// баристы всегда смена статуса (взять в работу / отменить), поэтому он синхронен
// между устройствами и перезапусками приложения без отдельного ack-поля.
// Realtime — WS канала точки, страховка — поллинг и тикер срочности (раз в 10 с:
// дедлайн переходит в «делать сейчас»/«промах» и без новых событий).

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.QueueEntryDto
import com.coffeely.mobile.api.hhmm
import com.coffeely.mobile.api.statusKdsRu
import com.coffeely.mobile.api.urgencyRu
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Цвета категорий борды: «тревожный» красный — новый/неотвеченный заказ,
// янтарный — в производстве, зелёный — готов, архив — без акцента (приглушён).
// Полупрозрачные подложки читаются и на светлой кремовой, и на тёмной теме.
private val KdsNew = Color(0xFFD32F2F)
private val KdsCooking = Color(0xFFE07B00)
private val KdsReady = Color(0xFF2E7D32)

private val KdsShape = RoundedCornerShape(18.dp)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BaristaScreen(api: ApiClient, onLogout: () -> Unit) {
    var entries by remember { mutableStateOf<List<QueueEntryDto>>(emptyList()) }
    var pointId by remember { mutableStateOf<String?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    var cancelCandidate by remember { mutableStateOf<QueueEntryDto?>(null) }
    val scope = rememberCoroutineScope()

    suspend fun refresh() {
        api.queue().fold(
            { entries = it.orders; pointId = it.point_id; error = null },
            { error = it.message },
        )
    }

    // Realtime: подписка на канал точки; при обрыве — переподключение через 2 с.
    LaunchedEffect(Unit) {
        while (true) {
            refresh()
            val pid = pointId
            if (pid == null) {
                delay(5000)
                continue
            }
            runCatching {
                api.pointEvents(pid).collect { refresh() }
            }
            delay(2000)
        }
    }

    // Тикер срочности: дедлайн «скоро» → «делать сейчас» → «промах» движется и
    // без событий, карточки обязаны светиться вовремя.
    LaunchedEffect(Unit) {
        while (true) {
            delay(10_000)
            refresh()
        }
    }

    fun act(entry: QueueEntryDto, next: String) {
        scope.launch {
            api.setStatus(entry.id, next).fold(
                { refresh() },
                { error = it.message; refresh() }, // 409 bad_transition — борда сходится сама
            )
        }
    }

    // Категории: активные по ходу производства, закрытые — архив дня.
    val fresh = entries.filter { it.status == "paid" }
    val cooking = entries.filter { it.status == "in_production" }
    val readyList = entries.filter { it.status == "ready" }
    val archive = entries.filter { it.status == "given_out" || it.status == "cancelled" }

    // Пульс категории «новые»: один общий на всю борду.
    val pulse = if (fresh.isEmpty()) 1f else rememberNewPulse()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Бариста · борда") },
                actions = { TextButton(onClick = onLogout) { Text("Выйти") } },
            )
        },
    ) { padding ->
        LazyColumn(
            Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            error?.let {
                item {
                    Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                }
            }

            item {
                KdsSectionHeader("НОВЫЕ · НЕТ ОТВЕТА", fresh.size, KdsNew, glow = fresh.isNotEmpty(), pulse = pulse)
            }
            if (fresh.isEmpty()) item { MutedNote("— пока тихо") }
            items(fresh, key = { it.id }) { entry ->
                KdsCard(entry, pulse, Modifier.animateItem(), ::act) { cancelCandidate = it }
            }

            item {
                KdsSectionHeader("В РАБОТЕ", cooking.size, KdsCooking, glow = cooking.isNotEmpty())
            }
            if (cooking.isEmpty()) item { MutedNote("— пусто") }
            items(cooking, key = { it.id }) { entry ->
                KdsCard(entry, 1f, Modifier.animateItem(), ::act) { cancelCandidate = it }
            }

            item {
                KdsSectionHeader("ГОТОВО · К ВЫДАЧЕ", readyList.size, KdsReady, glow = readyList.isNotEmpty())
            }
            if (readyList.isEmpty()) item { MutedNote("— пусто") }
            items(readyList, key = { it.id }) { entry ->
                KdsCard(entry, 1f, Modifier.animateItem(), ::act) { cancelCandidate = it }
            }

            item {
                KdsSectionHeader("АРХИВ ЗА ДЕНЬ", archive.size, accent = null)
            }
            if (archive.isEmpty()) item { MutedNote("— закрытых заказов пока нет") }
            items(archive, key = { it.id }) { entry ->
                KdsCard(entry, 1f, Modifier.animateItem(), ::act) { cancelCandidate = it }
            }
        }
    }

    // Отмена — второй ответ на новый заказ; подтверждение против случайного тапа.
    cancelCandidate?.let { candidate ->
        AlertDialog(
            onDismissRequest = { cancelCandidate = null },
            title = { Text("Отменить заказ №${candidate.number}?") },
            text = { Text("Гость увидит «отменён». Отмена засчитывается как ответ баристы.") },
            confirmButton = {
                TextButton(onClick = { cancelCandidate = null; act(candidate, "cancelled") }) {
                    Text("Отменить заказ")
                }
            },
            dismissButton = {
                TextButton(onClick = { cancelCandidate = null }) { Text("Оставить") }
            },
        )
    }
}

// rememberNewPulse — дышащая альфа: рамка и метка «новый» светятся-мигают,
// пока бариста не ответил на заказ.
@Composable
private fun rememberNewPulse(): Float {
    val transition = rememberInfiniteTransition(label = "kds-pulse")
    val alpha by transition.animateFloat(
        initialValue = 0.35f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(650), RepeatMode.Reverse),
        label = "kds-pulse-alpha",
    )
    return alpha
}

// KdsSectionHeader — заголовок категории с количеством и светящейся меткой.
@Composable
private fun KdsSectionHeader(
    title: String,
    count: Int,
    accent: Color?,
    glow: Boolean = false,
    pulse: Float = 1f,
) {
    val muted = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.55f)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (accent != null) {
            val dot = if (glow) accent.copy(alpha = pulse) else accent.copy(alpha = 0.35f)
            Box(Modifier.size(10.dp).background(dot, CircleShape))
        }
        Text(
            "$title · $count",
            style = MaterialTheme.typography.labelLarge,
            fontWeight = FontWeight.Bold,
            color = if (glow && accent != null) accent else muted,
        )
    }
}

// KdsCard — карточка заказа: цвет категории, срочность дедлайна, переход статуса.
@Composable
private fun KdsCard(
    entry: QueueEntryDto,
    pulse: Float,
    modifier: Modifier = Modifier,
    onAct: (QueueEntryDto, String) -> Unit,
    onCancel: (QueueEntryDto) -> Unit,
) {
    val closed = entry.status == "given_out" || entry.status == "cancelled"
    val accent = when (entry.status) {
        "paid" -> KdsNew
        "in_production" -> KdsCooking
        "ready" -> KdsReady
        else -> null // архив: приглушённая карточка без рамки
    }
    val container = when {
        accent == null -> MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f)
        else -> accent.copy(alpha = 0.13f)
    }
    // Рамка нового заказа мигает — «светится», пока на него не ответили.
    val border = when {
        accent == null -> Color.Transparent
        entry.status == "paid" -> accent.copy(alpha = pulse)
        else -> accent
    }
    val muted = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.55f)

    Card(
        modifier = modifier.fillMaxWidth().border(2.dp, border, KdsShape),
        shape = KdsShape,
        colors = CardDefaults.cardColors(containerColor = container),
        elevation = CardDefaults.cardElevation(defaultElevation = if (closed) 0.dp else 2.dp),
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "№${entry.number}",
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                    modifier = Modifier.weight(1f),
                )
                if (entry.urgency.isNotEmpty()) {
                    Text(
                        urgencyRu(entry.urgency),
                        color = urgencyColor(entry.urgency),
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = FontWeight.Bold,
                    )
                }
            }
            Text(
                "готовить к ${hhmm(entry.deadline_at)} · ${statusKdsRu(entry.status)}",
                style = MaterialTheme.typography.bodySmall,
                color = muted,
            )
            if (entry.items.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                Text(
                    entry.items.joinToString { "${it.title} ×${it.qty}" },
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (closed) muted else Color.Unspecified,
                )
            }
            Spacer(Modifier.height(10.dp))
            when (entry.status) {
                "paid" -> Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = { onAct(entry, "in_production") }, Modifier.weight(1f)) {
                        Text("Взять в работу")
                    }
                    TextButton(onClick = { onCancel(entry) }) { Text("Отменить") }
                }
                "in_production" -> Button(onClick = { onAct(entry, "ready") }, Modifier.fillMaxWidth()) {
                    Text("Готов")
                }
                "ready" -> Button(onClick = { onAct(entry, "given_out") }, Modifier.fillMaxWidth()) {
                    Text("Выдан")
                }
            }
        }
    }
}

@Composable
private fun MutedNote(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.5f),
    )
}

private fun urgencyColor(urgency: String): Color = when (urgency) {
    "overdue" -> Color(0xFFB3261E)
    "now" -> Color(0xFFB26A00)
    "soon" -> Color(0xFF6F4E37)
    else -> Color(0xFF2E7D32)
}
