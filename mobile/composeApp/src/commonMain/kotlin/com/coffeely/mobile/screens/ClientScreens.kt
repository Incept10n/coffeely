package com.coffeely.mobile.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.MenuItemDto
import com.coffeely.mobile.api.OrderDto
import com.coffeely.mobile.api.OrderItemRef
import com.coffeely.mobile.api.PointDto
import com.coffeely.mobile.api.WindowDto
import com.coffeely.mobile.api.hhmm
import com.coffeely.mobile.api.statusRu
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Клиентский флоу: точки → точка (окно + меню) → заказ → статус.
@Composable
fun ClientFlow(api: ApiClient, onLogout: () -> Unit) {
    var point by remember { mutableStateOf<PointDto?>(null) }
    if (point == null) {
        PointsScreen(api, onOpen = { point = it }, onLogout = onLogout)
    } else {
        PointScreen(api, point!!, onBack = { point = null })
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PointsScreen(api: ApiClient, onOpen: (PointDto) -> Unit, onLogout: () -> Unit) {
    var points by remember { mutableStateOf<List<PointDto>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) {
        api.points().fold({ points = it.points }, { error = it.message })
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("coffeely · точки") },
                actions = { TextButton(onClick = onLogout) { Text("Выйти") } },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(16.dp)) {
            error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(points) { p ->
                    Card(Modifier.fillMaxWidth().clickable { onOpen(p) }) {
                        Column(Modifier.padding(16.dp)) {
                            Text(p.title, style = MaterialTheme.typography.titleMedium)
                            if (p.address.isNotBlank()) {
                                Text(p.address, style = MaterialTheme.typography.bodySmall)
                            }
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PointScreen(api: ApiClient, point: PointDto, onBack: () -> Unit) {
    var menu by remember { mutableStateOf<List<MenuItemDto>>(emptyList()) }
    var windows by remember { mutableStateOf<List<WindowDto>>(emptyList()) }
    var selectedWindow by remember { mutableStateOf<WindowDto?>(null) }
    var cart by remember { mutableStateOf<Map<String, Int>>(emptyMap()) }
    var order by remember { mutableStateOf<OrderDto?>(null) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        api.menu(point.id).fold({ menu = it.menu }, { error = it.message })
        api.windows(point.id).fold({
            windows = it.windows.filter { w -> !w.passed && w.free > 0 }
        }, { error = it.message })
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(point.title) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "назад")
                    }
                },
            )
        },
    ) { padding ->
        val placed = order
        if (placed != null) {
            OrderStatusCard(placed, api, onBackToPoints = onBack, Modifier.padding(padding))
            return@Scaffold
        }
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
        ) {
            Text("Окно выдачи", style = MaterialTheme.typography.titleMedium)
            Text(
                "свободных окон: ${windows.size}",
                style = MaterialTheme.typography.bodySmall,
            )
            Spacer(Modifier.height(8.dp))
            Row(
                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                windows.forEach { w ->
                    FilterChip(
                        selected = selectedWindow == w,
                        onClick = { selectedWindow = w },
                        label = { Text("${hhmm(w.start)}–${hhmm(w.end)}") },
                    )
                }
            }
            Spacer(Modifier.height(16.dp))
            Text("Меню", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(8.dp))
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.weight(1f)) {
                items(menu) { item ->
                    val qty = cart[item.id] ?: 0
                    Card(Modifier.fillMaxWidth()) {
                        Row(
                            Modifier.padding(12.dp).fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Column(Modifier.weight(1f)) {
                                Text(item.title, style = MaterialTheme.typography.bodyLarge)
                                Text(
                                    "${item.price_minor / 100} ₽ · ${item.prep_seconds / 60} мин",
                                    style = MaterialTheme.typography.bodySmall,
                                )
                            }
                            TextButton(onClick = {
                                if (qty > 0) {
                                    cart = cart + (item.id to qty - 1)
                                    if (qty - 1 == 0) cart = cart - item.id
                                }
                            }) { Text("−") }
                            Text("$qty")
                            TextButton(onClick = { cart = cart + (item.id to qty + 1) }) { Text("+") }
                        }
                    }
                }
            }
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            Spacer(Modifier.height(8.dp))
            Button(
                onClick = {
                    val w = selectedWindow ?: return@Button
                    scope.launch {
                        busy = true
                        error = null
                        api.createOrder(
                            point.id, w.start, w.end,
                            cart.entries.map { OrderItemRef(it.key, it.value) },
                        ).fold({ order = it }, { error = it.message })
                        busy = false
                    }
                },
                enabled = selectedWindow != null && cart.isNotEmpty() && !busy,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(if (busy) "Оформляем…" else "Заказать")
            }
        }
    }
}

// Статус заказа с точным временем готовности; поллинг каждые 5 секунд.
@Composable
private fun OrderStatusCard(order: OrderDto, api: ApiClient, onBackToPoints: () -> Unit, modifier: Modifier = Modifier) {
    var current by remember { mutableStateOf(order) }
    LaunchedEffect(order.id) {
        while (true) {
            api.order(current.id).fold({ current = it }, { /* v1: пропускаем сетевые сбои поллинга */ })
            delay(5000)
        }
    }
    Column(modifier = modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Spacer(Modifier.height(48.dp))
        Text("Заказ №${current.number}", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(8.dp))
        Text(
            "Готов к ${hhmm(current.deadline_at)}",
            style = MaterialTheme.typography.headlineSmall,
        )
        Spacer(Modifier.height(8.dp))
        Text("Статус: ${statusRu(current.status)}")
        Spacer(Modifier.height(16.dp))
        LinearProgressIndicator(
            modifier = Modifier.fillMaxWidth(),
            progress = {
                when (current.status) {
                    "paid" -> 0.25f
                    "in_production" -> 0.5f
                    "ready" -> 1f
                    "given_out" -> 1f
                    else -> 0f
                }
            },
        )
        Spacer(Modifier.height(32.dp))
        TextButton(onClick = onBackToPoints) { Text("К списку точек") }
    }
}
