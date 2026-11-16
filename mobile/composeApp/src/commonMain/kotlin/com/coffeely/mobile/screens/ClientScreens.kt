package com.coffeely.mobile.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.MenuItemDto
import com.coffeely.mobile.api.OrderDto
import com.coffeely.mobile.api.OrderItemRef
import com.coffeely.mobile.api.PointDto
import com.coffeely.mobile.api.hhmm
import com.coffeely.mobile.api.isActive
import com.coffeely.mobile.api.statusRu
import com.coffeely.mobile.api.waitMinutes
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Клиентский флоу v2: точки (с ожиданием) → меню → заказ (окно назначает точка)
// → статус заказа в реальном времени. «Мои заказы» — отдельным экраном.
@Composable
fun ClientFlow(api: ApiClient, onLogout: () -> Unit) {
    var point by remember { mutableStateOf<PointDto?>(null) }
    var showOrders by remember { mutableStateOf(false) }
    var placed by remember { mutableStateOf<OrderDto?>(null) }

    when {
        placed != null -> OrderStatusScreen(
            api = api,
            order = placed!!,
            onDone = {
                placed = null
                point = null
            },
            onOpenOrders = {
                placed = null
                point = null
                showOrders = true
            },
        )

        showOrders -> OrdersScreen(
            api = api,
            onBack = { showOrders = false },
            onOpen = { placed = it },
        )

        point != null -> PointScreen(
            api = api,
            point = point!!,
            onBack = { point = null },
            onOrdered = { placed = it },
            onOpenOrders = { showOrders = true; point = null },
        )

        else -> PointsScreen(
            api = api,
            onOpen = { point = it },
            onOpenOrders = { showOrders = true },
            onLogout = onLogout,
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PointsScreen(
    api: ApiClient,
    onOpen: (PointDto) -> Unit,
    onOpenOrders: () -> Unit,
    onLogout: () -> Unit,
) {
    var points by remember { mutableStateOf<List<PointDto>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }
    var loading by remember { mutableStateOf(true) }

    // Загрузка точек обновляется, пока экран открыт: ожидание меняется на глазах.
    LaunchedEffect(Unit) {
        while (true) {
            api.points().fold(
                { points = it.points; error = null },
                { error = it.message },
            )
            loading = false
            delay(10_000)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("coffeely") },
                actions = {
                    TextButton(onClick = onOpenOrders) { Text("Мои заказы") }
                    TextButton(onClick = onLogout) { Text("Выйти") }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp)) {
            Text(
                "Выберите кофейню — покажем, когда будет готов ваш кофе",
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(vertical = 8.dp),
            )
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            if (loading) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator()
                }
                return@Column
            }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(points) { p ->
                    PointCard(p, onOpen = { onOpen(p) })
                }
            }
        }
    }
}

@Composable
private fun PointCard(point: PointDto, onOpen: () -> Unit) {
    Card(Modifier.fillMaxWidth().clickable { onOpen() }) {
        Row(Modifier.padding(16.dp).fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(point.title, style = MaterialTheme.typography.titleMedium)
                if (point.address.isNotBlank()) {
                    Text(point.address, style = MaterialTheme.typography.bodySmall)
                }
                Spacer(Modifier.height(4.dp))
                Text(
                    when {
                        !point.load.is_open && point.load.opens_at.isNotBlank() ->
                            "закрыто · откроется в ${point.load.opens_at}"
                        !point.load.is_open -> "закрыто"
                        point.load.active_orders == 0 -> "свободно · кофе сделаем сразу"
                        else -> "сейчас в очереди ${point.load.active_orders} — " +
                            "ваш кофе примерно через ${waitMinutes(point.load.wait_seconds)} мин"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = if (point.load.is_open) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
            }
            Text("›", style = MaterialTheme.typography.headlineSmall)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PointScreen(
    api: ApiClient,
    point: PointDto,
    onBack: () -> Unit,
    onOrdered: (OrderDto) -> Unit,
    onOpenOrders: () -> Unit,
) {
    var menu by remember { mutableStateOf<List<MenuItemDto>>(emptyList()) }
    var cart by remember { mutableStateOf<Map<String, Int>>(emptyMap()) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(point.id) {
        api.menu(point.id).fold({ menu = it.menu }, { error = it.message })
    }

    val totalMinor = menu.sumOf { (cart[it.id] ?: 0) * it.price_minor }
    val totalItems = cart.values.sum()
    val ownPrepSeconds = menu.sumOf { (cart[it.id] ?: 0) * it.prep_seconds }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(point.title) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "назад")
                    }
                },
                actions = { TextButton(onClick = onOpenOrders) { Text("Мои заказы") } },
            )
        },
        bottomBar = {
            if (totalItems > 0) {
                // Отдельная поверхность + навбар-инсет: при edge-to-edge три кнопки
                // Android больше не ложатся поверх цены и кнопки «Заказать».
                Surface(
                    color = MaterialTheme.colorScheme.surfaceContainerHighest,
                    tonalElevation = 3.dp,
                ) {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .navigationBarsPadding()
                            .padding(16.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column(Modifier.weight(1f)) {
                            Text("${totalMinor / 100} ₽", style = MaterialTheme.typography.titleMedium)
                            Text(
                                "оплата в кофейне при получении",
                                style = MaterialTheme.typography.bodySmall,
                            )
                        }
                        Button(
                            onClick = {
                                scope.launch {
                                    busy = true
                                    error = null
                                    api.createOrder(
                                        point.id,
                                        cart.entries.map { OrderItemRef(it.key, it.value) },
                                    ).fold({ onOrdered(it) }, { error = it.message })
                                    busy = false
                                }
                            },
                            // Закрытая точка заказ не примет (сервер вернёт point_busy).
                            enabled = !busy && point.load.is_open,
                        ) {
                            Text(
                                when {
                                    busy -> "Оформляем…"
                                    !point.load.is_open -> "Закрыто"
                                    else -> "Заказать"
                                },
                            )
                        }
                    }
                }
            }
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp)) {
            Card(
                Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceVariant,
                ),
            ) {
                Column(Modifier.padding(16.dp)) {
                    val wait = point.load.wait_seconds + ownPrepSeconds
                    Text(
                        when {
                            !point.load.is_open && point.load.opens_at.isNotBlank() ->
                                "Сейчас закрыто, откроемся в ${point.load.opens_at}"
                            totalItems == 0 ->
                                "Сейчас очередь: ${point.load.active_orders} · " +
                                    "обычно готовим ${waitMinutes(wait)} мин"
                            else ->
                                "Ваш заказ будет готов примерно через ${waitMinutes(wait)} мин"
                        },
                        style = MaterialTheme.typography.bodyMedium,
                        fontWeight = FontWeight.Medium,
                    )
                    Spacer(Modifier.height(4.dp))
                    Text(
                        "Точное время готовности покажем сразу после заказа — " +
                            "и будем обновлять статус, пока вы идёте.",
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }
            Spacer(Modifier.height(12.dp))
            Text("Меню", style = MaterialTheme.typography.titleMedium)
            LazyColumn(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.weight(1f),
            ) {
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
                                    "${item.price_minor / 100} ₽ · ${waitMinutes(item.prep_seconds)} мин",
                                    style = MaterialTheme.typography.bodySmall,
                                )
                            }
                            if (qty > 0) {
                                TextButton(onClick = {
                                    cart = if (qty == 1) cart - item.id else cart + (item.id to qty - 1)
                                }) { Text("−") }
                            }
                            Text("$qty")
                            TextButton(onClick = { cart = cart + (item.id to qty + 1) }) { Text("+") }
                        }
                    }
                }
            }
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

// «Мои заказы»: активные сверху, история ниже; обновление каждые 5 секунд.
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun OrdersScreen(api: ApiClient, onBack: () -> Unit, onOpen: (OrderDto) -> Unit) {
    var orders by remember { mutableStateOf<List<OrderDto>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }
    var loading by remember { mutableStateOf(true) }

    LaunchedEffect(Unit) {
        while (true) {
            api.myOrders().fold(
                { orders = it.orders; error = null },
                { error = it.message },
            )
            loading = false
            delay(5_000)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Мои заказы") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "назад")
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(16.dp)) {
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            if (loading) {
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator()
                }
                return@Column
            }
            if (orders.isEmpty()) {
                Text(
                    "Заказов пока нет. Выберите кофейню и закажите кофе.",
                    style = MaterialTheme.typography.bodyMedium,
                )
                return@Column
            }
            LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(orders) { o ->
                    Card(Modifier.fillMaxWidth().clickable { onOpen(o) }) {
                        Column(Modifier.padding(16.dp)) {
                            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    "№${o.number}",
                                    style = MaterialTheme.typography.titleMedium,
                                    modifier = Modifier.weight(1f),
                                )
                                StatusBadge(o.status)
                            }
                            if (o.point_title.isNotBlank()) {
                                Text(o.point_title, style = MaterialTheme.typography.bodySmall)
                            }
                            Text(
                                if (isActive(o.status)) {
                                    "Готов к ${hhmm(o.deadline_at)} · ${o.total_minor / 100} ₽"
                                } else {
                                    "${statusRu(o.status)} · ${o.total_minor / 100} ₽"
                                },
                                style = MaterialTheme.typography.bodyMedium,
                            )
                        }
                    }
                }
            }
        }
    }
}

// Экран активного заказа: статус приходит мгновенно (WebSocket) + страховочный поллинг.
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun OrderStatusScreen(
    api: ApiClient,
    order: OrderDto,
    onDone: () -> Unit,
    onOpenOrders: () -> Unit,
) {
    var current by remember { mutableStateOf(order) }

    LaunchedEffect(order.id) {
        // страховка на случай, если WS не поднялся (мобильная сеть, прокси)
        while (true) {
            api.order(current.id).fold({ current = it }, { /* сеть моргнула — повторим */ })
            delay(5_000)
        }
    }
    LaunchedEffect(order.id) {
        while (true) {
            runCatching {
                api.myEvents().collect { event ->
                    val updated = event.order ?: return@collect
                    if (updated.id == current.id) current = updated
                }
            }
            delay(2_000)
        }
    }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Заказ №${current.number}") },
                actions = { TextButton(onClick = onOpenOrders) { Text("Мои заказы") } },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier.fillMaxSize().padding(padding).padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Spacer(Modifier.height(24.dp))
            Text(
                if (current.status == "ready") "Кофе готов" else "Готовим ваш кофе",
                style = MaterialTheme.typography.headlineSmall,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                if (current.status == "ready" || current.status == "given_out") {
                    "Заберите на стойке: заказ №${current.number}"
                } else {
                    "Будет готов к ${hhmm(current.deadline_at)}"
                },
                style = MaterialTheme.typography.titleLarge,
            )
            Spacer(Modifier.height(24.dp))
            LinearProgressIndicator(
                modifier = Modifier.fillMaxWidth(),
                progress = { progressOf(current.status) },
            )
            Spacer(Modifier.height(16.dp))
            StatusBadge(current.status)
            if (current.items.isNotEmpty()) {
                Spacer(Modifier.height(24.dp))
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(16.dp)) {
                        current.items.forEach { item ->
                            Text(
                                "${item.title} ×${item.qty}",
                                style = MaterialTheme.typography.bodyMedium,
                            )
                        }
                        Spacer(Modifier.height(8.dp))
                        Text(
                            "Итого ${current.total_minor / 100} ₽ · оплата в кофейне",
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
            Spacer(Modifier.weight(1f))
            TextButton(onClick = onDone) { Text("К списку точек") }
        }
    }
}

@Composable
private fun StatusBadge(status: String) {
    val color = when (status) {
        "ready" -> Color(0xFF2E7D32)
        "in_production" -> Color(0xFF1565C0)
        "given_out" -> MaterialTheme.colorScheme.onSurfaceVariant
        "cancelled" -> MaterialTheme.colorScheme.error
        else -> Color(0xFF6D4C41)
    }
    Box(
        Modifier
            .background(color.copy(alpha = 0.12f), MaterialTheme.shapes.small)
            .padding(horizontal = 10.dp, vertical = 4.dp),
    ) {
        Text(statusRu(status), color = color, style = MaterialTheme.typography.labelMedium)
    }
}

private fun progressOf(status: String): Float = when (status) {
    "created", "paid" -> 0.15f
    "in_production" -> 0.55f
    "ready", "given_out" -> 1f
    else -> 0f
}
