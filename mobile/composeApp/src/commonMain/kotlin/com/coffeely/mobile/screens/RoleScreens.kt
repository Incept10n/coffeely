package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun RoleShell(
    title: String,
    subtitle: String,
    onLogout: () -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(title) },
                actions = {
                    TextButton(onClick = onLogout) { Text("Выйти") }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Top,
        ) {
            Text(subtitle, style = MaterialTheme.typography.bodyMedium)
            Spacer(Modifier.height(16.dp))
            content()
        }
    }
}

// Проверка связи с dev-бэкендом: вызов /readyz — видно, что приложение
// и сервер (БД включительно) реально связаны.
@Composable
private fun ApiCheckItem() {
    val scope = rememberCoroutineScope()
    val api = remember { ApiClient() }
    var result by remember { mutableStateOf<String?>(null) }

    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Button(onClick = {
            scope.launch {
                result = api.ready().fold(
                    { "readyz: ${it.status}, db=${it.db}" },
                    { "нет связи с бэкендом: ${it.message}" },
                )
            }
        }) {
            Text("Проверить dev-бэкенд")
        }
        result?.let {
            Text(it, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
fun ClientScreen(onLogout: () -> Unit) {
    RoleShell(
        title = "Клиент",
        subtitle = "Точки, меню, окна выдачи, заказ — веха M4",
        onLogout = onLogout,
    ) {
        ApiCheckItem()
    }
}

@Composable
fun BaristaScreen(onLogout: () -> Unit) {
    RoleShell(
        title = "Бариста · KDS",
        subtitle = "Очередь заказов с дедлайнами — веха M3",
        onLogout = onLogout,
    ) {
        ApiCheckItem()
    }
}
