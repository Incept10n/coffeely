package com.coffeely.mobile.api

// Access-токен текущей сессии (v1: в памяти процесса; хранение между запусками — позже).
object TokenStore {
    @Volatile
    var access: String? = null

    fun clear() {
        access = null
    }
}
