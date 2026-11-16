package com.coffeely.mobile.ui

import androidx.compose.runtime.Composable

// Итог входа через Яндекс ID на устройстве.
sealed interface YandexSignInResult {
    data class Success(val token: String) : YandexSignInResult
    data object Cancelled : YandexSignInResult
    data class Failure(val message: String) : YandexSignInResult
}

// Кнопка «Войти с Яндексом». На Android — официальный Yandex LoginSDK:
// нативный экран входа (или Chrome Tab/WebView, если приложений Яндекса нет),
// токен возвращается прямо в приложение — без редиректов и deep link.
// Вызывающий меняет токен на сессию coffeely через POST /auth/yandex/token.
@Composable
expect fun YandexSignInButton(
    enabled: Boolean,
    onResult: (YandexSignInResult) -> Unit,
)
