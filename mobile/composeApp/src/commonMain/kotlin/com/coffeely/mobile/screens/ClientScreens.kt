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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ExitToApp
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.ShoppingCart
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.AddonDto
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.MenuItemDto
import com.coffeely.mobile.api.OrderDto
import com.coffeely.mobile.api.OrderItemRef
import com.coffeely.mobile.api.PointDto
import com.coffeely.mobile.api.hhmm
import com.coffeely.mobile.api.isActive
import com.coffeely.mobile.api.statusRu
import com.coffeely.mobile.api.waitMinutes
import com.coffeely.mobile.ui.ThemeDialog
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Строка корзины: количество и выбранные добавки (id из каталога точки).
private data class CartLine(val qty: Int, val addonIds: Set<String> = emptySet())

// Клиентский флоу v2: главный экран (карта / список точек) → меню → заказ
// (окно назначает точка) → статус заказа в реальном времени. «Мои заказы»
// открываются из бургер-меню главного экрана.
@Composable
fun ClientFlow(api: ApiClient, onLogout: () -> Unit) {
    var point by remember { mutableStateOf<PointDto?>(null) }
    var showOrders by remember { mutableStateOf(false) }
    var placed by remember { mutableStateOf<OrderDto?>(null) }

    when {
        placed != null -> OrderStatusScreen(
            api = api,
            order = placed!!,
            // «К списку точек» всегда ведёт на главный экран со списком:
            // заказ мог быть открыт и из «Моих заказов», и с карты — во всех
            // случаях закрываем и заказ, и экран заказов, и экран точки.
            onDone = {
                placed = null
                point = null
                showOrders = false
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

// Два режима главного экрана. Всё остальное (заказы, тема, «о программе»,
// выход) убрано в бургер-меню: в шапке остаются только эти две области.
private enum class HomeTab { MAP, LIST }

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
    var tab by remember { mutableStateOf(HomeTab.LIST) }
    var showAbout by remember { mutableStateOf(false) }
    var showTheme by remember { mutableStateOf(false) }
    val drawerState = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()

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

    // Закрываем меню и сразу выполняем действие: после навигации этот экран
    // может исчезнуть, и анимация закрытия уже не нужна.
    fun closeDrawer(action: () -> Unit) {
        scope.launch { drawerState.close() }
        action()
    }

    ModalNavigationDrawer(
        drawerState = drawerState,
        // Карта занимает весь экран и тянется пальцем; свайп от левого края
        // открывал бы меню вместо прокрутки карты. Открываем только бургером.
        gesturesEnabled = false,
        drawerContent = {
            ModalDrawerSheet {
                Column(
                    Modifier
                        .fillMaxWidth()
                        .background(MaterialTheme.colorScheme.primaryContainer)
                        .padding(horizontal = 24.dp, vertical = 28.dp),
                ) {
                    Text(
                        "coffeely",
                        style = MaterialTheme.typography.headlineSmall,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onPrimaryContainer,
                    )
                    Text(
                        "предзаказ кофе к точному времени",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.8f),
                    )
                }
                Spacer(Modifier.height(12.dp))
                DrawerItem("Мои заказы", Icons.Filled.ShoppingCart) {
                    closeDrawer(onOpenOrders)
                }
                DrawerItem("О программе", Icons.Filled.Info) {
                    closeDrawer { showAbout = true }
                }
                DrawerItem("Тема оформления", Icons.Filled.Settings) {
                    closeDrawer { showTheme = true }
                }
                HorizontalDivider(
                    Modifier.padding(horizontal = 24.dp, vertical = 12.dp),
                    color = MaterialTheme.colorScheme.outlineVariant,
                )
                DrawerItem(
                    label = "Выйти",
                    icon = Icons.AutoMirrored.Filled.ExitToApp,
                    tint = MaterialTheme.colorScheme.error,
                ) {
                    closeDrawer(onLogout)
                }
            }
        },
    ) {
        Scaffold(
            topBar = {
                Column {
                    TopAppBar(
                        title = {
                            Text("coffeely", fontWeight = FontWeight.Bold)
                        },
                        navigationIcon = {
                            IconButton(onClick = { scope.launch { drawerState.open() } }) {
                                Icon(Icons.Filled.Menu, contentDescription = "Меню")
                            }
                        },
                    )
                    HomeTabSwitch(selected = tab, onSelect = { tab = it })
                    error?.let {
                        Text(
                            it,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                        )
                    }
                }
            },
        ) { padding ->
            Box(Modifier.fillMaxSize().padding(padding)) {
                when (tab) {
                    HomeTab.MAP -> PointsMapTab(
                        points = points,
                        onOpenPoint = onOpen,
                        modifier = Modifier.fillMaxSize(),
                    )

                    HomeTab.LIST -> PointsList(
                        points = points,
                        onOpen = onOpen,
                        modifier = Modifier.fillMaxSize(),
                    )
                }
                if (loading && points.isEmpty()) {
                    CircularProgressIndicator(Modifier.align(Alignment.Center))
                }
            }
        }
    }

    ThemeDialog(open = showTheme, onDismiss = { showTheme = false })
    if (showAbout) MapAboutDialog(onClose = { showAbout = false })
}

// Две выделяющиеся области в шапке: карта и список. Раньше это были
// текстовые кнопки в actions, вместе с темой, заказами и выходом.
@Composable
private fun HomeTabSwitch(selected: HomeTab, onSelect: (HomeTab) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp)
            .padding(bottom = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        HomeTabButton(
            label = "Точка на карте",
            icon = Icons.Filled.Place,
            selected = selected == HomeTab.MAP,
            modifier = Modifier.weight(1f),
        ) { onSelect(HomeTab.MAP) }
        HomeTabButton(
            label = "Список точек",
            icon = Icons.AutoMirrored.Filled.List,
            selected = selected == HomeTab.LIST,
            modifier = Modifier.weight(1f),
        ) { onSelect(HomeTab.LIST) }
    }
}

@Composable
private fun HomeTabButton(
    label: String,
    icon: ImageVector,
    selected: Boolean,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
) {
    val container = if (selected) {
        MaterialTheme.colorScheme.primary
    } else {
        MaterialTheme.colorScheme.surfaceContainerHighest
    }
    val content = if (selected) {
        MaterialTheme.colorScheme.onPrimary
    } else {
        MaterialTheme.colorScheme.onSurfaceVariant
    }

    Surface(
        modifier = modifier
            .height(46.dp)
            .clickable(onClick = onClick),
        shape = RoundedCornerShape(14.dp),
        color = container,
        contentColor = content,
    ) {
        Row(
            modifier = Modifier.fillMaxSize(),
            horizontalArrangement = Arrangement.Center,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(label, style = MaterialTheme.typography.labelLarge, maxLines = 1)
        }
    }
}

@Composable
private fun DrawerItem(
    label: String,
    icon: ImageVector,
    tint: Color = MaterialTheme.colorScheme.onSurface,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 24.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
        Spacer(Modifier.width(16.dp))
        Text(label, style = MaterialTheme.typography.bodyLarge, color = tint)
    }
}

@Composable
private fun PointsList(
    points: List<PointDto>,
    onOpen: (PointDto) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier.padding(horizontal = 16.dp)) {
        Text(
            "Выберите кофейню — покажем, когда будет готов ваш кофе",
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(vertical = 8.dp),
        )
        if (points.isEmpty()) {
            Text(
                "Рядом пока нет точек на карте — загляните во вкладку «Точка на карте».",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(points) { p ->
                PointCard(p, onOpen = { onOpen(p) })
            }
        }
    }
}

@Composable
private fun PointCard(point: PointDto, onOpen: () -> Unit) {
    Card(Modifier.fillMaxWidth().clickable { onOpen() }) {
        Row(Modifier.padding(16.dp).fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                PointTitle(point)
                if (point.address.isNotBlank()) {
                    Spacer(Modifier.height(2.dp))
                    Text(point.address, style = MaterialTheme.typography.bodySmall)
                }
                Spacer(Modifier.height(4.dp))
                Text(
                    when {
                        !point.load.is_open && point.load.on_break && point.load.opens_at.isNotBlank() ->
                            "перерыв · откроется в ${point.load.opens_at}"
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
    var cart by remember { mutableStateOf<Map<String, CartLine>>(emptyMap()) }
    var addonTarget by remember { mutableStateOf<MenuItemDto?>(null) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(point.id) {
        api.menu(point.id).fold(
            { menu = it.menu },
            { error = it.message },
        )
    }

    // Добавки принадлежат товару: у каждого напитка свой набор.
    fun selectedAddons(item: MenuItemDto, line: CartLine?): List<AddonDto> =
        item.addons.filter { it.id in (line?.addonIds ?: emptySet()) }

    val totalMinor = menu.sumOf { item ->
        val line = cart[item.id] ?: return@sumOf 0L
        val addonMinor = selectedAddons(item, line).sumOf { it.price_minor }
        line.qty * (item.price_minor + addonMinor)
    }
    val totalItems = cart.values.sumOf { it.qty }
    val ownPrepSeconds = menu.sumOf { item ->
        val line = cart[item.id] ?: return@sumOf 0
        val addonPrep = selectedAddons(item, line).sumOf { it.prep_seconds }
        line.qty * (item.prep_seconds + addonPrep)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        if (point.brand_name.isNotBlank()) {
                            Text(
                                point.brand_name.trim(),
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.primary,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        Text(
                            point.title,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                },
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
                                        cart.map { (menuItemId, line) ->
                                            OrderItemRef(menuItemId, line.qty, line.addonIds.toList())
                                        },
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
                            !point.load.is_open && point.load.on_break && point.load.opens_at.isNotBlank() ->
                                "Сейчас перерыв до ${point.load.opens_at}"
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
                        "Бариста возьмёт заказ в работу — точное время покажем " +
                            "на статусе заказа.",
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
                    val line = cart[item.id]
                    val qty = line?.qty ?: 0
                    val chosen = selectedAddons(item, line)
                    Card(Modifier.fillMaxWidth()) {
                        Column(Modifier.padding(12.dp)) {
                            Row(
                                Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Column(Modifier.weight(1f)) {
                                    Text(item.title, style = MaterialTheme.typography.bodyLarge)
                                    Text(
                                        "${item.price_minor / 100} ₽ · ${waitMinutes(item.prep_seconds)} мин",
                                        style = MaterialTheme.typography.bodySmall,
                                    )
                                    if (chosen.isNotEmpty()) {
                                        Text(
                                            "добавки: ${chosen.joinToString { it.title }} " +
                                                "+${chosen.sumOf { it.price_minor } / 100} ₽",
                                            style = MaterialTheme.typography.bodySmall,
                                            color = MaterialTheme.colorScheme.primary,
                                        )
                                    }
                                }
                                if (qty > 0) {
                                    TextButton(onClick = {
                                        cart = if (qty == 1) {
                                            cart - item.id
                                        } else {
                                            cart + (item.id to line!!.copy(qty = qty - 1))
                                        }
                                    }) { Text("−") }
                                }
                                Text("$qty")
                                TextButton(onClick = {
                                    cart = cart + (item.id to
                                        (line ?: CartLine(0)).copy(qty = qty + 1))
                                }) { Text("+") }
                            }
                            if (item.addons.isNotEmpty()) {
                                TextButton(onClick = { addonTarget = item }) {
                                    Text(if (chosen.isEmpty()) "Добавить добавки" else "Изменить добавки")
                                }
                            }
                        }
                    }
                }
            }
            error?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        }
    }

    addonTarget?.let { target ->
        val current = cart[target.id] ?: CartLine(0)
        AddonPickerDialog(
            menuItemTitle = target.title,
            addons = target.addons,
            selected = current.addonIds,
            onDismiss = { addonTarget = null },
            onConfirm = { selected ->
                val line = cart[target.id]
                cart = if (line == null) {
                    cart + (target.id to CartLine(1, selected))
                } else {
                    cart + (target.id to line.copy(addonIds = selected))
                }
                addonTarget = null
            },
        )
    }
}

// AddonPickerDialog — выбор добавок к одному напитку. Добавки применяются ко
// всему количеству этого напитка в корзине.
@Composable
private fun AddonPickerDialog(
    menuItemTitle: String,
    addons: List<AddonDto>,
    selected: Set<String>,
    onDismiss: () -> Unit,
    onConfirm: (Set<String>) -> Unit,
) {
    var chosen by remember(menuItemTitle) { mutableStateOf(selected) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Добавки к «$menuItemTitle»") },
        text = {
            Column {
                addons.forEach { addon ->
                    Row(
                        Modifier.fillMaxWidth().clickable {
                            chosen = if (addon.id in chosen) chosen - addon.id else chosen + addon.id
                        },
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Checkbox(
                            checked = addon.id in chosen,
                            onCheckedChange = { checked ->
                                chosen = if (checked) chosen + addon.id else chosen - addon.id
                            },
                        )
                        Text(
                            addon.title,
                            style = MaterialTheme.typography.bodyMedium,
                            modifier = Modifier.weight(1f),
                        )
                        Text(
                            if (addon.price_minor > 0) "+${addon.price_minor / 100} ₽" else "бесплатно",
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(chosen) }) { Text("Готово") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Отмена") }
        },
    )
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
                            if (o.status == "paid") {
                                Text(
                                    if (o.accept_in_seconds > 0) {
                                        "бариста возьмёт через ${waitMinutes(o.accept_in_seconds.toInt())} мин"
                                    } else {
                                        "бариста берёт заказ в работу"
                                    },
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.primary,
                                )
                            }
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
        // Кнопка закреплена внизу: длинный чек прокручивается, но «К списку
        // точек» всегда на экране, а не уезжает за нижнюю границу.
        bottomBar = {
            Box(
                Modifier
                    .fillMaxWidth()
                    .navigationBarsPadding()
                    .padding(horizontal = 24.dp, vertical = 8.dp),
            ) {
                TextButton(onClick = onDone, modifier = Modifier.fillMaxWidth()) {
                    Text("К списку точек")
                }
            }
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Spacer(Modifier.height(24.dp))
            Text(
                when (current.status) {
                    "paid" -> "Заказ принят"
                    "ready" -> "Кофе готов"
                    "given_out" -> "Заказ выдан"
                    "cancelled" -> "Заказ отменён"
                    else -> "Готовим ваш кофе"
                },
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
            if (current.status == "paid") {
                Spacer(Modifier.height(12.dp))
                Text(
                    baristaEtaText(current.accept_in_seconds, current.accept_by_at),
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.Medium,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
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
                            val addons = if (item.addons.isEmpty()) {
                                ""
                            } else {
                                " · " + item.addons.joinToString { it.title }
                            }
                            Text(
                                "${item.title} ×${item.qty}$addons",
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
        }
    }
}

// baristaEtaText — ETA «бариста возьмёт заказ»: сколько осталось и к какому
// времени (в поясе телефона). Время считает сервер, поэтому пояс не важен.
private fun baristaEtaText(secondsLeft: Long, acceptBy: String): String {
    val until = when {
        acceptBy.isBlank() -> ""
        else -> " (к ${hhmm(acceptBy)})"
    }
    return when {
        secondsLeft > 0 ->
            "Бариста возьмёт заказ примерно через ${waitMinutes(secondsLeft.toInt())} мин$until"
        secondsLeft > -300 ->
            "Бариста уже должен взять заказ — вот-вот начнёт$until"
        else -> "Бариста задерживается — уточните на стойке"
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
