package com.coffeely.mobile

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.TokenStore
import com.coffeely.mobile.api.UserDto
import com.coffeely.mobile.screens.BaristaScreen
import com.coffeely.mobile.screens.ClientFlow
import com.coffeely.mobile.screens.LoginScreen
import com.coffeely.mobile.ui.CoffeelyTheme

@Composable
fun App() {
    var user by remember { mutableStateOf<UserDto?>(null) }
    val api = remember { ApiClient() }

    fun logout() {
        TokenStore.clear()
        user = null
    }

    CoffeelyTheme {
        val current = user
        when {
            current == null -> LoginScreen(api) { user = it }
            current.role == "barista" -> BaristaScreen(api, onLogout = ::logout)
            else -> ClientFlow(api, onLogout = ::logout)
        }
    }
}
