package com.coffeely.mobile.api

import kotlinx.serialization.Serializable

@Serializable
data class UserDto(val id: String, val email: String, val role: String)

@Serializable
data class TokensDto(
    val access_token: String,
    val refresh_token: String,
    val token_type: String = "Bearer",
)

@Serializable
data class AuthResponse(val user: UserDto, val tokens: TokensDto)

// Текущая загрузка точки: сколько ждать, если заказать сейчас.
@Serializable
data class PointLoadDto(
    val wait_seconds: Int = 0,
    val active_orders: Int = 0,
    val is_open: Boolean = false,
    val opens_at: String = "",
)

@Serializable
data class PointDto(
    val id: String,
    val title: String,
    val address: String = "",
    val timezone: String = "",
    val load: PointLoadDto = PointLoadDto(),
)

@Serializable
data class PointsResponse(val points: List<PointDto> = emptyList())

@Serializable
data class MenuItemDto(val id: String, val title: String, val price_minor: Long, val prep_seconds: Int)

@Serializable
data class MenuResponse(val menu: List<MenuItemDto> = emptyList())

@Serializable
data class OrderItemDto(
    val menu_item_id: String,
    val title: String = "",
    val qty: Int,
    val price_minor: Long = 0,
)

@Serializable
data class OrderDto(
    val id: String,
    val number: Long,
    val point_id: String,
    val point_title: String = "",
    val client_id: String = "",
    val status: String,
    val pickup_slot_start: String,
    val pickup_slot_end: String,
    val deadline_at: String,
    val total_minor: Long = 0,
    val items: List<OrderItemDto> = emptyList(),
)

@Serializable
data class OrdersResponse(val orders: List<OrderDto> = emptyList())

// Запись KDS-очереди: поля заказа + срочность от движка дедлайна.
@Serializable
data class QueueEntryDto(
    val id: String,
    val number: Long,
    val point_id: String,
    val status: String,
    val pickup_slot_start: String,
    val pickup_slot_end: String,
    val deadline_at: String,
    val total_minor: Long = 0,
    val items: List<OrderItemDto> = emptyList(),
    val until_deadline_seconds: Long = 0,
    val urgency: String = "",
)

@Serializable
data class QueueResponse(val point_id: String, val orders: List<QueueEntryDto> = emptyList())

// Событие realtime-канала: {"type":"order_created"|"order_status","order":{...}}
@Serializable
data class WsEvent(val type: String, val order: OrderDto? = null)

@Serializable
data class ErrorResponse(val code: String = "", val message: String = "")

@Serializable
data class RegisterRequest(val email: String, val password: String)

@Serializable
data class LoginRequest(val email: String, val password: String)

@Serializable
data class CreateOrderRequest(
    val point_id: String,
    val items: List<OrderItemRef>,
)

@Serializable
data class OrderItemRef(val menu_item_id: String, val qty: Int)

@Serializable
data class StatusRequest(val status: String)

@Serializable
data class RefreshRequest(val refresh_token: String)

@Serializable
data class LogoutRequest(val refresh_token: String)

// Человекочитаемые подписи статусов и срочности.

// Что происходит с заказом — так, как это видит гость.
fun statusRu(status: String): String = when (status) {
    "created" -> "создан"
    "paid" -> "принят — бариста возьмёт в работу"
    "in_production" -> "готовится"
    "ready" -> "готов — забирайте"
    "given_out" -> "выдан"
    "cancelled" -> "отменён"
    else -> status
}

// Активные заказы — те, за которыми стоит следить.
fun isActive(status: String): Boolean = status in setOf("created", "paid", "in_production", "ready")

fun urgencyRu(urgency: String): String = when (urgency) {
    "overdue" -> "ПРОМАХ ДЕДЛАЙНА"
    "now" -> "делать сейчас"
    "soon" -> "скоро"
    "later" -> "время есть"
    else -> ""
}

// "2026-11-15T17:52:00+03:00" → "17:52" (RFC3339 от бэкенда всегда с секундами).
fun hhmm(iso: String): String = if (iso.length >= 16) iso.substring(11, 16) else iso

// Секунды ожидания → «~4 мин» (округление вверх, минимум 1 мин).
fun waitMinutes(seconds: Int): Int = if (seconds <= 0) 1 else (seconds + 59) / 60
