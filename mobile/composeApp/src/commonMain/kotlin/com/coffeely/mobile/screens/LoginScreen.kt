package com.coffeely.mobile.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import com.coffeely.mobile.api.ApiException
import com.coffeely.mobile.api.AuthResponse
import com.coffeely.mobile.api.TokenStore
import com.coffeely.mobile.api.UserDto
import com.coffeely.mobile.ui.ThemeModeButton
import com.coffeely.mobile.ui.YandexSignInButton
import com.coffeely.mobile.ui.YandexSignInResult
import kotlinx.coroutines.launch

// Вход/регистрация через реальный API бэкенда (M2): email+пароль, argon2id.
// После регистрации бэкенд отправляет код на email: вход открывается только
// после ввода кода (экран подтверждения — в этом же файле).
//
// Кнопка «Регистрация» нажимается всегда, даже с пустыми полями: она открывает
// отдельное окно регистрации. Пользователь не упирается в серую кнопку, а сразу
// видит, что нужно ввести.
@Composable
fun LoginScreen(api: ApiClient, onLoggedIn: (UserDto) -> Unit) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var showRegister by remember { mutableStateOf(false) }

    // Непустой email — открыто окно подтверждения: пользователь вводит код из письма.
    var verifyTarget by remember { mutableStateOf<String?>(null) }
    var verifyNotice by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    // Вход через Яндекс ID: кнопка есть только если сервер его настроил
    // (YANDEX_CLIENT_ID задан), иначе вход остаётся email+пароль.
    var yandexAvailable by remember { mutableStateOf(false) }
    var yandexBusy by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        api.authProviders().onSuccess { yandexAvailable = it.yandex }
    }

    fun enter(resp: AuthResponse) {
        TokenStore.set(resp.tokens.access_token, resp.tokens.refresh_token)
        showRegister = false
        verifyTarget = null
        onLoggedIn(resp.user)
    }

    fun login(emailValue: String, passwordValue: String) {
        scope.launch {
            busy = true
            error = null
            api.login(emailValue.trim(), passwordValue).fold(
                { resp -> enter(resp) },
                { e ->
                    if (e is ApiException && e.code == "email_not_verified") {
                        // Пароль верный, но email не подтверждён: показываем ввод кода.
                        showRegister = false
                        verifyNotice = "Введите код из письма. Если письма нет — отправьте код ещё раз."
                        verifyTarget = emailValue.trim()
                    } else {
                        error = e.message ?: "ошибка сети"
                    }
                },
            )
            busy = false
        }
    }

    fun register(emailValue: String, passwordValue: String) {
        scope.launch {
            busy = true
            error = null
            api.register(emailValue.trim(), passwordValue).fold(
                { resp ->
                    val tokens = resp.tokens
                    if (tokens != null) {
                        // Почта не настроена (dev): регистрация сразу выдаёт сессию.
                        TokenStore.set(tokens.access_token, tokens.refresh_token)
                        showRegister = false
                        onLoggedIn(resp.user)
                    } else {
                        showRegister = false
                        verifyNotice = if (resp.email_sent) {
                            "Код отправлен на ${resp.user.email}. Введите его ниже."
                        } else {
                            "Письмо не отправилось. Нажмите «Отправить код ещё раз»."
                        }
                        verifyTarget = resp.user.email.ifBlank { emailValue.trim() }
                    }
                },
                { error = it.message ?: "ошибка сети" },
            )
            busy = false
        }
    }

    // Токен из LoginSDK меняем на сессию coffeely (сервер сам проверит его в Яндексе).
    fun loginWithYandex(token: String) {
        scope.launch {
            yandexBusy = true
            error = null
            api.loginWithYandex(token).fold(
                { resp -> enter(resp) },
                { e ->
                    error = when {
                        e is ApiException && e.code == "yandex_barista" ->
                            "Вход через Яндекс — только для клиентов. Бариста входит по email и паролю, который выдал владелец."
                        e is ApiException && e.code == "yandex_owner" ->
                            "Владельцу — в личный кабинет на сайте."
                        else -> e.message ?: "ошибка сети"
                    }
                },
            )
            yandexBusy = false
        }
    }

    // Фон — общий кремовый (а не белый): под прозрачными системными панелями видно
    // именно его, поэтому полоса навигации больше не выделяется белым.
    // safeDrawingPadding + imePadding: панели и клавиатура не перекрывают поля и кнопку,
    // а на низком экране форма прокручивается вместо обрезки.
    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        BoxWithConstraints(
            modifier = Modifier
                .fillMaxSize()
                .safeDrawingPadding()
                .imePadding(),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .heightIn(min = maxHeight)
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

                // Неактивная кнопка красится в цвет onSurface с прозрачностью; при
                // кофейной схеме это тёплый оттенок, а не фиолетовая подсветка
                // baseline Material 3.
                if (busy) {
                    CircularProgressIndicator()
                } else {
                    Button(
                        onClick = { login(email, password) },
                        modifier = Modifier.fillMaxWidth(),
                        enabled = email.isNotBlank() && password.length >= 8,
                    ) {
                        Text("Войти")
                    }
                    // Яндекс ID — дополнительный способ: кнопка появляется, когда
                    // сервер сообщил, что он настроен. Токен выдаёт LoginSDK.
                    if (yandexAvailable) {
                        Spacer(Modifier.height(12.dp))
                        Text(
                            "или",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Spacer(Modifier.height(8.dp))
                        YandexSignInButton(enabled = !yandexBusy) { result ->
                            when (result) {
                                is YandexSignInResult.Success -> loginWithYandex(result.token)
                                is YandexSignInResult.Failure -> error = result.message
                                YandexSignInResult.Cancelled -> Unit
                            }
                        }
                    }
                    // Всегда активна: пустые поля — не повод блокировать переход
                    // к регистрации, окно само покажет, что нужно заполнить.
                    TextButton(
                        onClick = {
                            error = null
                            showRegister = true
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text("Регистрация нового клиента")
                    }
                    ThemeModeButton(modifier = Modifier.fillMaxWidth())
                }
                if (!showRegister && verifyTarget == null) {
                    error?.let {
                        Spacer(Modifier.height(12.dp))
                        Text(
                            it,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        }
    }

    if (showRegister) {
        var regEmail by remember { mutableStateOf(email) }
        var regPassword by remember { mutableStateOf(password) }
        val canSubmit = regEmail.isNotBlank() && regPassword.length >= 8 && !busy

        AlertDialog(
            onDismissRequest = { if (!busy) showRegister = false },
            title = { Text("Регистрация клиента") },
            text = {
                Column(modifier = Modifier.fillMaxWidth()) {
                    Text(
                        "Аккаунт нужен, чтобы видеть свои заказы и точное время готовности. " +
                            "На email придёт код подтверждения.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(12.dp))
                    OutlinedTextField(
                        value = regEmail,
                        onValueChange = { regEmail = it },
                        label = { Text("Email") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Spacer(Modifier.height(8.dp))
                    OutlinedTextField(
                        value = regPassword,
                        onValueChange = { regPassword = it },
                        label = { Text("Пароль (мин. 8 символов)") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    error?.let {
                        Spacer(Modifier.height(8.dp))
                        Text(
                            it,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            },
            confirmButton = {
                TextButton(
                    onClick = { register(regEmail, regPassword) },
                    enabled = canSubmit,
                ) {
                    Text(if (busy) "Создаём…" else "Создать аккаунт")
                }
            },
            dismissButton = {
                TextButton(
                    onClick = { showRegister = false },
                    enabled = !busy,
                ) {
                    Text("Отмена")
                }
            },
        )
    }

    verifyTarget?.let { target ->
        var code by remember(target) { mutableStateOf("") }
        var verifyBusy by remember(target) { mutableStateOf(false) }
        var verifyError by remember(target) { mutableStateOf<String?>(null) }
        var notice by remember(target) { mutableStateOf(verifyNotice) }

        AlertDialog(
            onDismissRequest = { if (!verifyBusy) verifyTarget = null },
            title = { Text("Подтверждение email") },
            text = {
                Column(modifier = Modifier.fillMaxWidth()) {
                    Text(
                        target,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(8.dp))
                    notice?.let {
                        Text(it, style = MaterialTheme.typography.bodySmall)
                        Spacer(Modifier.height(8.dp))
                    }
                    OutlinedTextField(
                        value = code,
                        onValueChange = { code = it.filter(Char::isDigit).take(6) },
                        label = { Text("Код из письма") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    verifyError?.let {
                        Spacer(Modifier.height(8.dp))
                        Text(
                            it,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        scope.launch {
                            verifyBusy = true
                            verifyError = null
                            api.verifyEmail(target, code).fold(
                                { resp -> enter(resp) },
                                { verifyError = it.message ?: "ошибка сети" },
                            )
                            verifyBusy = false
                        }
                    },
                    enabled = code.length == 6 && !verifyBusy,
                ) {
                    Text(if (verifyBusy) "Проверяем…" else "Подтвердить")
                }
            },
            dismissButton = {
                Row {
                    TextButton(
                        onClick = {
                            scope.launch {
                                verifyBusy = true
                                verifyError = null
                                api.resendVerification(target).fold(
                                    {
                                        notice = if (it.email_sent) {
                                            "Код отправлен повторно."
                                        } else {
                                            "Письмо не отправилось — попробуйте позже."
                                        }
                                    },
                                    { verifyError = it.message ?: "ошибка сети" },
                                )
                                verifyBusy = false
                            }
                        },
                        enabled = !verifyBusy,
                    ) {
                        Text("Отправить код")
                    }
                    TextButton(
                        onClick = { verifyTarget = null },
                        enabled = !verifyBusy,
                    ) {
                        Text("Отмена")
                    }
                }
            },
        )
    }
}
