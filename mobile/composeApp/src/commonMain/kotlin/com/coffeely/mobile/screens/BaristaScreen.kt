package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
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
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.QueueEntryDto
import com.coffeely.mobile.api.hhmm
import com.coffeely.mobile.api.statusRu
import com.coffeely.mobile.api.urgencyRu
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// KDS баристы: очередь с дедлайнами, переходы статусов, поллинг каждые 5 секунд
// (push через WebSocket подключим следующим шагом; API уже отдаёт /ws/points/{id}).
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BaristaScreen(api: ApiClient, onLogout: () -> Unit) {
    var orders by remember { mutableStateOf<List<QueueEntryDto>>(emptyList()) }
    var pointId by remember { mutableStateOf<String?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    suspend fun refresh() {
        api.queue().fold(
            { orders = it.orders; pointId = it.point_id; error = null },
            { error = it.message },
        )
    }

    // Realtime: подписка на канал точки; при обрыве — переподключение через 2 с.
    // Поллинг оставлен только как страховка (раз в 30 тиков ~ без нагрузки).
    LaunchedEffect(Unit) {
        while (true) {
            refresh()
            val pid = pointId
            if (pid == null) {
                delay(5000)
                continue
            }
            runCatching {
                api.pointEvents(pid).collect {
                    refresh()
                }
            }
            delay(2000)
        }
    }

    fun act(entry: QueueEntryDto, next: String) {
        scope.launch {
            api.setStatus(entry.id, next).fold(
                { refresh() },
                { error = it.message },
            )
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Бариста · очередь") },
                actions = { TextButton(onClick = onLogout) { Text("Выйти") } },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(16.dp)) {
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                Spacer(Modifier.height(8.dp))
            }
            if (orders.isEmpty()) {
                Text("Очередь пуста", style = MaterialTheme.typography.bodyMedium)
            }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(orders) { entry ->
                    Card(Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(16.dp)) {
                            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    "№${entry.number} — к ${hhmm(entry.deadline_at)}",
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.weight(1f),
                                )
                                Text(
                                    urgencyRu(entry.urgency),
                                    color = urgencyColor(entry.urgency),
                                    style = MaterialTheme.typography.labelMedium,
                                )
                            }
                            Text(statusRu(entry.status), style = MaterialTheme.typography.bodySmall)
                            if (entry.items.isNotEmpty()) {
                                Text(
                                    entry.items.joinToString { "${it.title} ×${it.qty}" },
                                    style = MaterialTheme.typography.bodyMedium,
                                )
                            }
                            Spacer(Modifier.height(8.dp))
                            when (entry.status) {
                                "paid" -> Button(onClick = { act(entry, "in_production") }) { Text("Взять в работу") }
                                "in_production" -> Button(onClick = { act(entry, "ready") }) { Text("Готов") }
                                "ready" -> Button(onClick = { act(entry, "given_out") }) { Text("Выдан") }
                            }
                        }
                    }
                }
            }
        }
    }
}

private fun urgencyColor(urgency: String): Color = when (urgency) {
    "overdue" -> Color(0xFFB3261E)
    "now" -> Color(0xFFB26A00)
    "soon" -> Color(0xFF6F4E37)
    else -> Color(0xFF2E7D32)
}
