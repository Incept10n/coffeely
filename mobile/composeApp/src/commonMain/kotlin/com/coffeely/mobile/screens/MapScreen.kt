package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.PointDto
import com.coffeely.mobile.api.PointLoadDto
import com.coffeely.mobile.api.waitMinutes
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

// Точка в том виде, в котором её ждёт страница /map.
@Serializable
private data class MapPoint(
    val id: String,
    val title: String,
    val address: String,
    val lat: Double,
    val lng: Double,
)

private val mapJson = Json { encodeDefaults = true }

/**
 * Карта активных точек. Список берём из GET /points (там уже есть lat/lng),
 * саму карту рисует веб-страница /map в WebView. Клик по пину открывает
 * карточку; «Выбрать кофейню» ведёт в существующий экран точки.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MapScreen(
    api: ApiClient,
    onBack: () -> Unit,
    onSelect: (PointDto) -> Unit,
) {
    var points by remember { mutableStateOf<List<PointDto>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var selected by remember { mutableStateOf<PointDto?>(null) }
    var reload by remember { mutableStateOf(0) }
    var about by remember { mutableStateOf(false) }

    LaunchedEffect(reload) {
        loading = true
        api.points().fold(
            { points = it.points; error = null },
            { error = it.message },
        )
        loading = false
    }

    val mapped = remember(points) {
        points.mapNotNull { p ->
            val lat = p.lat ?: return@mapNotNull null
            val lng = p.lng ?: return@mapNotNull null
            MapPoint(p.id, p.title, p.address, lat, lng)
        }
    }
    val json = remember(mapped) { mapJson.encodeToString(mapped) }
    val uri = LocalUriHandler.current

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Кофейни на карте") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Назад")
                    }
                },
                actions = {
                    TextButton(onClick = { about = true }) { Text("О программе") }
                },
            )
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            when {
                loading -> CircularProgressIndicator(Modifier.align(Alignment.Center))

                error != null -> Column(
                    modifier = Modifier.align(Alignment.Center).padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(error ?: "ошибка сети", style = MaterialTheme.typography.bodyMedium)
                    Button(onClick = { reload += 1 }) { Text("Повторить") }
                }

                mapped.isEmpty() -> Column(
                    modifier = Modifier.align(Alignment.Center).padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text("Пока ни одна точка не отмечена на карте")
                    Text(
                        "Координаты точки задаёт владелец в кабинете.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }

                else -> MapWebView(
                    pointsJson = json,
                    onPointClick = { id -> selected = points.find { it.id == id } },
                    modifier = Modifier.fillMaxSize(),
                )
            }

            selected?.let { p ->
                ShopCard(
                    point = p,
                    onClose = { selected = null },
                    onSelect = { onSelect(p) },
                    modifier = Modifier
                        .align(Alignment.BottomCenter)
                        .padding(16.dp)
                        .fillMaxWidth(),
                )
            }
        }
    }

    // Условие бесплатного Tiles API: ссылка на условия Яндекс Карт в разделе
    // «О программе». Открываем системным браузером.
    if (about) {
        AlertDialog(
            onDismissRequest = { about = false },
            title = { Text("О программе") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("coffeely — предзаказ кофе к точному времени готовности.")
                    Text("Карта: Яндекс Карты.")
                    TextButton(
                        onClick = {
                            runCatching {
                                uri.openUri("https://yandex.ru/legal/maps_termsofuse")
                            }
                        },
                    ) {
                        Text("Условия использования сервиса Яндекс.Карты")
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { about = false }) { Text("Закрыть") }
            },
        )
    }
}

@Composable
private fun ShopCard(
    point: PointDto,
    onClose: () -> Unit,
    onSelect: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        modifier = modifier,
        elevation = CardDefaults.cardElevation(defaultElevation = 6.dp),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    if (point.brand_name.isNotBlank()) {
                        Text(
                            point.brand_name,
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                    }
                    Text(point.title, style = MaterialTheme.typography.titleMedium)
                }
                TextButton(onClick = onClose) { Text("Закрыть") }
            }
            if (point.address.isNotBlank()) {
                Text(point.address, style = MaterialTheme.typography.bodySmall)
            }
            Text(loadLabel(point.load), style = MaterialTheme.typography.bodySmall)
            Button(onClick = onSelect, modifier = Modifier.fillMaxWidth()) {
                Text("Выбрать кофейню")
            }
        }
    }
}

private fun loadLabel(load: PointLoadDto): String = when {
    !load.is_open && load.on_break && load.opens_at.isNotBlank() ->
        "перерыв · откроется в ${load.opens_at}"

    !load.is_open && load.opens_at.isNotBlank() ->
        "закрыто · откроется в ${load.opens_at}"

    !load.is_open -> "закрыто"
    load.active_orders == 0 -> "свободно · кофе сделаем сразу"
    else -> "в очереди ${load.active_orders} · примерно ${waitMinutes(load.wait_seconds)} мин"
}
