package com.coffeely.mobile.api

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.plugins.websocket.WebSockets
import io.ktor.client.plugins.websocket.webSocket
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import io.ktor.http.isSuccess
import io.ktor.serialization.kotlinx.json.json
import io.ktor.websocket.Frame
import io.ktor.websocket.readText
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.serialization.json.Json

// Адрес API приходит из сборки (BuildConfig.API_BASE_URL): в CI — прод,
// локально — эмулятор (10.0.2.2 виден как хост-машина).
internal expect fun platformApiBaseUrl(): String

class ApiClient(baseUrl: String = platformApiBaseUrl()) {
    private val base = baseUrl.trimEnd('/')

    // http://host:8080 → ws://host:8080; https:// → wss://
    private val wsBase = when {
        base.startsWith("https://") -> "wss://" + base.removePrefix("https://")
        base.startsWith("http://") -> "ws://" + base.removePrefix("http://")
        else -> base
    }

    private val json = Json { ignoreUnknownKeys = true }

    private val client = HttpClient {
        install(ContentNegotiation) {
            json(this@ApiClient.json)
        }
        install(WebSockets)
    }

    // --- аутентификация ---

    suspend fun register(email: String, password: String): Result<AuthResponse> = runCatching {
        postJson("/auth/register", RegisterRequest(email, password))
    }

    suspend fun login(email: String, password: String): Result<AuthResponse> = runCatching {
        postJson("/auth/login", LoginRequest(email, password))
    }

    suspend fun refresh(refreshToken: String): Result<AuthResponse> = runCatching {
        postJson("/auth/refresh", RefreshRequest(refreshToken))
    }

    suspend fun me(): Result<UserDto> = runCatching { getJson("/auth/me") }

    suspend fun logout(refreshToken: String): Result<Unit> = runCatching {
        val resp = client.post("$base/auth/logout") {
            contentType(ContentType.Application.Json)
            setBody(LogoutRequest(refreshToken))
            TokenStore.access?.let { header("Authorization", "Bearer $it") }
        }
        if (!resp.status.isSuccess()) throw ApiException("не удалось выйти (${resp.status.value})")
    }

    // --- клиентский флоу ---

    suspend fun points(): Result<PointsResponse> = runCatching { getJson("/points") }

    suspend fun menu(pointId: String): Result<MenuResponse> =
        runCatching { getJson("/points/$pointId/menu") }

    // Окно производства не передаётся: его назначает точка по текущей очереди.
    suspend fun createOrder(pointId: String, items: List<OrderItemRef>): Result<OrderDto> =
        runCatching { postJson("/orders", CreateOrderRequest(pointId, items)) }

    suspend fun order(id: String): Result<OrderDto> = runCatching { getJson("/orders/$id") }

    suspend fun myOrders(): Result<OrdersResponse> = runCatching { getJson("/orders") }

    // --- бариста (KDS) ---

    suspend fun queue(): Result<QueueResponse> = runCatching { getJson("/barista/queue") }

    // Realtime-канал точки (WebSocket): новые заказы и смены статусов без поллинга.
    // Поток завершается при обрыве; вызывающий переподключается.
    fun pointEvents(pointId: String): Flow<WsEvent> = flow {
        client.webSocket(
            urlString = wsBase + "/ws/points/" + pointId,
            request = {
                TokenStore.access?.let { header("Authorization", "Bearer $it") }
            },
        ) {
            for (frame in incoming) {
                if (frame is Frame.Text) {
                    val event = runCatching {
                        json.decodeFromString<WsEvent>(frame.readText())
                    }.getOrNull()
                    if (event != null) emit(event)
                }
            }
        }
    }

    // Realtime-канал клиента: статусы его заказов.
    fun myEvents(): Flow<WsEvent> = flow {
        client.webSocket(
            urlString = wsBase + "/ws/orders",
            request = {
                TokenStore.access?.let { header("Authorization", "Bearer $it") }
            },
        ) {
            for (frame in incoming) {
                if (frame is Frame.Text) {
                    val event = runCatching {
                        json.decodeFromString<WsEvent>(frame.readText())
                    }.getOrNull()
                    if (event != null) emit(event)
                }
            }
        }
    }

    suspend fun setStatus(orderId: String, status: String): Result<OrderDto> = runCatching {
        postJson("/barista/orders/$orderId/status", StatusRequest(status))
    }

    // --- транспорт ---

    private suspend inline fun <reified T> getJson(path: String): T {
        val resp = client.get("$base$path") {
            TokenStore.access?.let { header("Authorization", "Bearer $it") }
        }
        return resp.bodyOrThrow()
    }

    private suspend inline fun <reified T> postJson(path: String, body: Any): T {
        val resp = client.post("$base$path") {
            contentType(ContentType.Application.Json)
            setBody(body)
            TokenStore.access?.let { header("Authorization", "Bearer $it") }
        }
        return resp.bodyOrThrow()
    }

    private suspend inline fun <reified T> io.ktor.client.statement.HttpResponse.bodyOrThrow(): T {
        if (!status.isSuccess()) {
            val msg = runCatching { body<ErrorResponse>().message }
                .getOrDefault("ошибка ${status.value}")
            throw ApiException(msg)
        }
        return body<T>()
    }
}

class ApiException(message: String) : Exception(message)
