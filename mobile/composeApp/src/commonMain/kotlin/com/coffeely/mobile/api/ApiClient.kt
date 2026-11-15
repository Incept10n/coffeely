package com.coffeely.mobile.api

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import io.ktor.http.isSuccess
import io.ktor.serialization.kotlinx.json.json
import kotlinx.serialization.json.Json

// dev: эмулятор Android видит host-машину как 10.0.2.2.
// На реальном устройстве замени на IP компьютера в той же Wi-Fi-сети.
const val DEFAULT_BASE_URL = "http://10.0.2.2:8080"

class ApiClient(baseUrl: String = DEFAULT_BASE_URL) {
    private val base = baseUrl.trimEnd('/')

    private val json = Json { ignoreUnknownKeys = true }

    private val client = HttpClient {
        install(ContentNegotiation) {
            json(this@ApiClient.json)
        }
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

    suspend fun windows(pointId: String): Result<WindowsResponse> =
        runCatching { getJson("/points/$pointId/windows") }

    suspend fun createOrder(
        pointId: String,
        slotStart: String,
        slotEnd: String,
        items: List<OrderItemRef>,
    ): Result<OrderDto> = runCatching {
        postJson(
            "/orders",
            CreateOrderRequest(pointId, slotStart, slotEnd, items),
        )
    }

    suspend fun order(id: String): Result<OrderDto> = runCatching { getJson("/orders/$id") }

    // --- бариста (KDS) ---

    suspend fun queue(): Result<QueueResponse> = runCatching { getJson("/barista/queue") }

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
