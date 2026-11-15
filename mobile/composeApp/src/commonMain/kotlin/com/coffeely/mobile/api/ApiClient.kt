package com.coffeely.mobile.api

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.serialization.kotlinx.json.json
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

@Serializable
data class HealthResponse(
    val status: String,
    val service: String? = null,
)

@Serializable
data class ReadyResponse(
    val status: String,
    val db: Boolean = false,
)

// dev: эмулятор Android видит host-машину как 10.0.2.2.
// На реальном устройстве замени на IP компьютера в той же Wi-Fi-сети.
const val DEFAULT_BASE_URL = "http://10.0.2.2:8080"

class ApiClient(baseUrl: String = DEFAULT_BASE_URL) {
    private val base = baseUrl.trimEnd('/')

    private val client = HttpClient {
        install(ContentNegotiation) {
            json(Json { ignoreUnknownKeys = true })
        }
    }

    suspend fun health(): Result<HealthResponse> = runCatching {
        client.get("$base/healthz").body<HealthResponse>()
    }

    suspend fun ready(): Result<ReadyResponse> = runCatching {
        client.get("$base/readyz").body<ReadyResponse>()
    }
}
