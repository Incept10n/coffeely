package com.coffeely.mobile.api

import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

// Часовой пояс телефона; если система его не отдала — Москва (аудитория
// сервиса в РФ). UTC не показываем никогда: это не «время кофейни».
private val fallbackZone: ZoneId = runCatching { ZoneId.of("Europe/Moscow") }
    .getOrElse { ZoneId.systemDefault() }

private val hhmmFormatter: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm")

internal actual fun hhmmLocal(iso: String): String {
    if (iso.isBlank()) return ""
    return runCatching {
        val instant = runCatching { Instant.parse(iso) }
            .getOrElse { OffsetDateTime.parse(iso).toInstant() }
        val zone = runCatching { ZoneId.systemDefault() }.getOrElse { fallbackZone }
        hhmmFormatter.format(instant.atZone(zone))
    }.getOrElse {
        // Нестандартная строка: если в ней есть смещение, показываем его как есть;
        // иначе — московское время не выдумываем, отдаём исходный фрагмент.
        if (iso.length >= 16) iso.substring(11, 16) else iso
    }
}
