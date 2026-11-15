package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp

// Заготовка входа: форма email+пароль. Реальная аутентификация — веха M2
// (argon2id, JWT, Yandex ID на бэкенде), здесь пока только UI.
@Composable
fun LoginScreen(
    onEnterAsClient: () -> Unit,
    onEnterAsBarista: () -> Unit,
) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }

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
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(12.dp))
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("Пароль") },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(8.dp))
            Text(
                "Вход заработает в M2 (email + Yandex ID)",
                style = MaterialTheme.typography.bodySmall,
            )
            Spacer(Modifier.height(24.dp))

            Button(
                onClick = onEnterAsClient,
                modifier = Modifier.fillMaxWidth(),
                enabled = false,
            ) {
                Text("Войти (скоро, M2)")
            }
            Spacer(Modifier.height(12.dp))
            OutlinedButton(
                onClick = onEnterAsClient,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text("Демо: я клиент")
            }
            OutlinedButton(
                onClick = onEnterAsBarista,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text("Демо: я бариста")
            }
        }
    }
}
