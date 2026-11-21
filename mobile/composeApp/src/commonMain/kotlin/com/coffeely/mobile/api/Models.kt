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

// Регистрация может требовать подтверждения email: tokens придут после
// ввода кода из письма (/auth/verify-email).
@Serializable
data class RegisterResponse(
    val user: UserDto,
    val tokens: TokensDto? = null,
    val verification_required: Boolean = false,
    val email_sent: Boolean = false,
)

@Serializable
data class VerifyEmailRequest(val email: String, val code: String)

@Serializable
data class ResendVerificationRequest(val email: String)

@Serializable
data class ResendVerificationResponse(
    val email_sent: Boolean = false,
    val retry_after_seconds: Int = 0,
)

// Текущая загрузка точки: сколько ждать, если заказать сейчас.
@Serializable
data class PointLoadDto(
    val wait_seconds: Int = 0,
    val active_orders: Int = 0,
    val is_open: Boolean = false,
    val on_break: Boolean = false,
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

// Добавка к напитку: сироп, овсяное молоко, доп. шот.
@Serializable
data class AddonDto(
    val id: String,
    val title: String,
    val price_minor: Long = 0,
    val prep_seconds: Int = 0,
)

@Serializable
data class MenuResponse(
    val menu: List<MenuItemDto> = emptyList(),
    val addons: List<AddonDto> = emptyList(),
)

@Serializable
data class OrderAddonDto(
    val addon_id: String = "",
    val title: String = "",
    val price_minor: Long = 0,
)

@Serializable
data class OrderItemDto(
    val menu_item_id: String,
    val title: String = "",
    val qty: Int,
    val price_minor: Long = 0,
    val addons: List<OrderAddonDto> = emptyList(),
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
    val accept_by_at: String = "",
    val accept_in_seconds: Long = 0,
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
    val accept_by_at: String = "",
    val accept_in_seconds: Long = 0,
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
data class OrderItemRef(
    val menu_item_id: String,
    val qty: Int,
    val addons: List<String> = emptyList(),
)

@Serializable
data class StatusRequest(val status: String)

@Serializable
data class RefreshRequest(val refresh_token: String)

@Serializable
data class LogoutRequest(val refresh_token: String)

// Какие способы входа настроены на сервере (приложение прячет кнопки).
@Serializable
data class ProvidersResponse(val yandex: Boolean = false)

// Токен из LoginSDK: сервер проверит его в Яндексе и выдаст свою сессию.
@Serializable
data class YandexTokenRequest(val token: String)

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

// Короткие подписи статусов для KDS-борды баристы: «на что ответил / на что нет»
// читается с первого взгляда.
fun statusKdsRu(status: String): String = when (status) {
    "paid" -> "новый · нет ответа"
    "in_production" -> "в работе"
    "ready" -> "готов"
    "given_out" -> "выдан"
    "cancelled" -> "отменён"
    else -> statusRu(status)
}

fun urgencyRu(urgency: String): String = when (urgency) {
    "overdue" -> "ПРОМАХ ДЕДЛАЙНА"
    "now" -> "делать сейчас"
    "soon" -> "скоро"
    "later" -> "время есть"
    else -> ""
}

// RFC3339 от бэкенда → "17:52" в часовом поясе телефона. Бэкенд отдаёт время
// в UTC (timestamptz), поэтому substring(11,16) показывал бы UTC — так нельзя.
// Если пояс определить не удалось, показываем московское время, но не UTC.
fun hhmm(iso: String): String = hhmmLocal(iso)

// Секунды ожидания → «~4 мин» (округление вверх, минимум 1 мин).
fun waitMinutes(seconds: Int): Int = if (seconds <= 0) 1 else (seconds + 59) / 60
