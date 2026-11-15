package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.coffeely.mobile.api.ApiClient
import com.coffeely.mobile.api.TokenStore
import com.coffeely.mobile.api.UserDto
import kotlinx.coroutines.launch

// Вход/регистрация через реальный API бэкенда (M2): email+пароль, argon2id.
// Сессия сохраняется на устройстве (TokenStore), Yandex ID — следующий шаг.
@Composable
fun LoginScreen(api: ApiClient, onLoggedIn: (UserDto) -> Unit) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    fun auth(register: Boolean) {
        scope.launch {
            busy = true
            error = null
            val result = if (register) api.register(email.trim(), password) else api.login(email.trim(), password)
            result.fold(
                { resp ->
                    TokenStore.set(resp.tokens.access_token, resp.tokens.refresh_token)
                    onLoggedIn(resp.user)
                },
                { error = it.message ?: "ошибка сети" },
            )
            busy = false
        }
    }

    Surface(modifier = Modifier.fillMaxSize()) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Text("coffeely", style = MaterialTheme.typography.displaySmall)
            Text(
                "кофе к точному времени готовности",
                style = MaterialTheme.typography.bodyMedium,
            )
            Spacer(Modifier.height(32.dp))

            OutlinedTextField(
                value = email,
                onValueChange = { email = it },
                label = { Text("Email") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("Пароль (мин. 8 символов)") },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(16.dp))

            if (busy) {
                CircularProgressIndicator()
            } else {
                Button(
                    onClick = { auth(register = false) },
                    modifier = Modifier.fillMaxWidth(),
                    enabled = email.isNotBlank() && password.length >= 8,
                ) {
                    Text("Войти")
                }
                TextButton(
                    onClick = { auth(register = true) },
                    modifier = Modifier.fillMaxWidth(),
                    enabled = email.isNotBlank() && password.length >= 8,
                ) {
                    Text("Регистрация нового клиента")
                }
            }
            error?.let {
                Spacer(Modifier.height(12.dp))
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
