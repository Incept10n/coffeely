package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.PointDto
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
 * Вкладка «Точка на карте» главного экрана: Leaflet с тайлами Яндекс Карт
 * внутри WebView плюс карточка выбранной точки. Список точек уже загружен
 * экраном, сама карта остаётся живой и без единой точки — по ней всё равно
 * можно нажать «Где я» и увидеть своё местоположение.
 */
@Composable
fun PointsMapTab(
    points: List<PointDto>,
    onOpenPoint: (PointDto) -> Unit,
    modifier: Modifier = Modifier,
) {
    var selectedId by remember { mutableStateOf<String?>(null) }

    val mapped = remember(points) {
        points.mapNotNull { p ->
            val lat = p.lat ?: return@mapNotNull null
            val lng = p.lng ?: return@mapNotNull null
            MapPoint(p.id, p.title, p.address, lat, lng)
        }
    }
    val json = remember(mapped) { mapJson.encodeToString(mapped) }
    val selected = remember(points, selectedId) { points.find { it.id == selectedId } }

    Box(modifier.fillMaxSize()) {
        MapWebView(
            pointsJson = json,
            onPointClick = { id -> selectedId = id },
            modifier = Modifier.fillMaxSize(),
        )

        if (mapped.isEmpty()) {
            Surface(
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .padding(16.dp)
                    .fillMaxWidth(),
                shape = RoundedCornerShape(16.dp),
                color = MaterialTheme.colorScheme.surfaceContainerHigh,
                shadowElevation = 2.dp,
            ) {
                Text(
                    "Пока ни одна точка не отмечена на карте. Нажмите «Где я», " +
                        "чтобы увидеть своё местоположение.",
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(14.dp),
                )
            }
        }

        selected?.let { p ->
            ShopCard(
                point = p,
                onClose = { selectedId = null },
                onSelect = { onOpenPoint(p) },
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .navigationBarsPadding()
                    .padding(16.dp)
                    .fillMaxWidth(),
            )
        }
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
                verticalAlignment = Alignment.Top,
            ) {
                PointTitle(point, Modifier.weight(1f))
                TextButton(onClick = onClose) { Text("Закрыть") }
            }
            if (point.address.isNotBlank()) {
                Text(point.address, style = MaterialTheme.typography.bodySmall)
            }
            Text(loadLabel(point), style = MaterialTheme.typography.bodySmall)
            Button(onClick = onSelect, modifier = Modifier.fillMaxWidth()) {
                Text("Выбрать кофейню")
            }
        }
    }
}

/**
 * Условие бесплатного Tiles API: ссылка на условия Яндекс Карт обязана быть
 * доступна из приложения. Открываем её из бургер-меню («О программе»).
 */
@Composable
fun MapAboutDialog(onClose: () -> Unit) {
    val uri = LocalUriHandler.current
    AlertDialog(
        onDismissRequest = onClose,
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
            TextButton(onClick = onClose) { Text("Закрыть") }
        },
    )
}

private fun loadLabel(point: PointDto): String {
    val load = point.load
    return when {
        !load.is_open && load.on_break && load.opens_at.isNotBlank() ->
            "перерыв · откроется в ${load.opens_at}"

        !load.is_open && load.opens_at.isNotBlank() ->
            "закрыто · откроется в ${load.opens_at}"

        !load.is_open -> "закрыто"
        load.active_orders == 0 -> "свободно · кофе сделаем сразу"
        else -> "в очереди ${load.active_orders} · примерно ${waitMinutes(load.wait_seconds)} мин"
    }
}
