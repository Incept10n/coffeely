package com.coffeely.mobile.api

// Сессия в памяти процесса + запись в хранилище платформы (SharedPreferences
// на Android), чтобы при следующем запуске клиент сразу видел свои заказы.
object TokenStore {
    @Volatile
    var access: String? = null
        private set

    @Volatile
    var refresh: String? = null
        private set

    fun set(accessToken: String, refreshToken: String) {
        access = accessToken
        refresh = refreshToken
        SessionStorage.save(accessToken, refreshToken)
    }

    // Восстановление из хранилища при старте приложения.
    fun restore() {
        val saved = SessionStorage.load() ?: return
        access = saved.access
        refresh = saved.refresh
    }

    fun clear() {
        access = null
        refresh = null
        SessionStorage.clear()
    }
}

// Сохранённая сессия (access истекает через 15 минут — обновляем по refresh).
data class StoredSession(val access: String, val refresh: String)

internal expect object SessionStorage {
    fun save(access: String, refresh: String)
    fun load(): StoredSession?
    fun clear()
}
