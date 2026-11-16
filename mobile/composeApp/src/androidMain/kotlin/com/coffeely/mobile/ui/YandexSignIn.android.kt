package com.coffeely.mobile.ui

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.BuildConfig
import com.yandex.authsdk.YandexAuthLoginOptions
import com.yandex.authsdk.YandexAuthOptions
import com.yandex.authsdk.YandexAuthResult
import com.yandex.authsdk.YandexAuthSdk

@Composable
actual fun YandexSignInButton(enabled: Boolean, onResult: (YandexSignInResult) -> Unit) {
    // Сборка без ClientID (локальная или до настройки CI) — кнопки нет:
    // без идентификатора приложения Яндекс экран входа не откроется.
    if (BuildConfig.YANDEX_CLIENT_ID.isBlank()) return

    val context = LocalContext.current
    val sdk = remember { YandexAuthSdk.create(YandexAuthOptions(context)) }
    val launcher = rememberLauncherForActivityResult(sdk.contract) { result ->
        when (result) {
            is YandexAuthResult.Success -> onResult(YandexSignInResult.Success(result.token.value))
            is YandexAuthResult.Failure -> {
                // Ошибки SDK — машинные коды (CONNECTION_ERROR, SECURITY_ERROR…);
                // показываем их рядом с понятным текстом, иначе не отладить.
                val codes = result.exception.errors.filter { it.isNotBlank() }.joinToString(", ")
                val message = if (codes.isEmpty()) {
                    "Не удалось войти через Яндекс"
                } else {
                    "Не удалось войти через Яндекс ($codes)"
                }
                onResult(YandexSignInResult.Failure(message))
            }
            YandexAuthResult.Cancelled -> onResult(YandexSignInResult.Cancelled)
        }
    }

    // Разделитель живёт здесь, а не в общем экране: без ClientID кнопки нет,
    // и одинокое «или» над формой выглядело бы поломкой.
    Column(
        modifier = Modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            "или",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(8.dp))
        OutlinedButton(
            onClick = { launcher.launch(YandexAuthLoginOptions()) },
            enabled = enabled,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text("Войти с Яндексом")
        }
    }
}
