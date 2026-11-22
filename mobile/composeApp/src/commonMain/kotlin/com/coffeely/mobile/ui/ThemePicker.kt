package com.coffeely.mobile.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

// Компактный выбор темы: «Тема» в шапке/на экране входа открывает три варианта.
// Выбор сохраняется на устройстве (ThemeStorage) и применяется сразу.
@Composable
fun ThemeModeButton(modifier: Modifier = Modifier, label: String = "Тема") {
    var open by remember { mutableStateOf(false) }

    TextButton(onClick = { open = true }, modifier = modifier) {
        Text(label)
    }

    ThemeDialog(open = open, onDismiss = { open = false })
}

// Диалог выбора темы. Публичный: бургер-меню на главном экране держит его
// открытие в своём состоянии и показывает тем же composable.
@Composable
fun ThemeDialog(open: Boolean, onDismiss: () -> Unit) {
    if (!open) return

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Тема оформления") },
        text = {
            Column(Modifier.fillMaxWidth()) {
                ThemeOption(
                    title = "Как в системе",
                    subtitle = "Светлая или тёмная — по настройке телефона",
                    selected = ThemePreference.mode == ThemeMode.SYSTEM,
                ) { ThemePreference.select(ThemeMode.SYSTEM); onDismiss() }
                ThemeOption(
                    title = "Светлая",
                    subtitle = "Кремовый фон, кофейный текст",
                    selected = ThemePreference.mode == ThemeMode.LIGHT,
                ) { ThemePreference.select(ThemeMode.LIGHT); onDismiss() }
                ThemeOption(
                    title = "Тёмная",
                    subtitle = "Тёмный кофе, приглушённые панели",
                    selected = ThemePreference.mode == ThemeMode.DARK,
                ) { ThemePreference.select(ThemeMode.DARK); onDismiss() }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text("Закрыть") }
        },
    )
}

@Composable
private fun ThemeOption(
    title: String,
    subtitle: String,
    selected: Boolean,
    onSelect: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onSelect)
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = onSelect)
        Spacer(Modifier.width(4.dp))
        Column(Modifier.fillMaxWidth()) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(
                subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
