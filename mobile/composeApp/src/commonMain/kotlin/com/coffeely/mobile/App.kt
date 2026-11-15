package com.coffeely.mobile

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.coffeely.mobile.screens.BaristaScreen
import com.coffeely.mobile.screens.ClientScreen
import com.coffeely.mobile.screens.LoginScreen
import com.coffeely.mobile.ui.CoffeelyTheme

// Роли из схемы БД (backend/migrations): client / barista (+ owner под CRM, в мобилке его нет).
enum class Role { Client, Barista }

@Composable
fun App() {
    var role by remember { mutableStateOf<Role?>(null) }

    CoffeelyTheme {
        when (val current = role) {
            null -> LoginScreen(
                onEnterAsClient = { role = Role.Client },
                onEnterAsBarista = { role = Role.Barista },
            )
            Role.Client -> ClientScreen(onLogout = { role = null })
            Role.Barista -> BaristaScreen(onLogout = { role = null })
        }
    }
}
