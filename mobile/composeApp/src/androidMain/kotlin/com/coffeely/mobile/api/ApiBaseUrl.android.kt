package com.coffeely.mobile.api

import com.coffeely.mobile.BuildConfig

// Адрес API для Android-сборки: приходит из Gradle (BuildConfig.API_BASE_URL).
// CI передаёт прод-адрес, локальная сборка по умолчанию смотрит на эмулятор.
internal actual fun platformApiBaseUrl(): String = BuildConfig.API_BASE_URL
