package com.coffeely.mobile

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
    var booting by remember { mutableStateOf(true) }
    val api = remember { ApiClient() }

    fun logout() {
        TokenStore.clear()
        user = null
    }

    // При старте поднимаем сохранённую сессию: гость сразу видит свои заказы.
    LaunchedEffect(Unit) {
        TokenStore.restore()
        if (TokenStore.access != null) {
            api.me().fold(
                { user = it },
                {
                    // access истёк (15 минут) — обновляем по refresh-токену.
                    val refresh = TokenStore.refresh
                    if (refresh == null) {
                        TokenStore.clear()
                    } else {
                        api.refresh(refresh).fold(
                            { resp ->
                                TokenStore.set(resp.tokens.access_token, resp.tokens.refresh_token)
                                user = resp.user
                            },
                            { TokenStore.clear() },
                        )
                    }
                },
            )
        }
        booting = false
    }

    CoffeelyTheme {
        // Фон тянем на всё окно, включая полосы статуса и навигации: системные панели
        // прозрачные (edge-to-edge), цвет под ними берётся отсюда — крем или тёмный,
        // без белой полосы под тремя кнопками Android.
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
        ) {
            val current = user
            when {
                booting -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator()
                }

                current == null -> LoginScreen(api) { user = it }
                current.role == "barista" -> BaristaScreen(api, onLogout = ::logout)
                else -> ClientFlow(api, onLogout = ::logout)
            }
        }
    }
}
