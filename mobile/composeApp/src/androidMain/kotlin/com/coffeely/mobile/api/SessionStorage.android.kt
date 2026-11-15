package com.coffeely.mobile.api

import android.content.Context
import android.content.SharedPreferences

// Хранение сессии на Android. init вызывается из MainActivity до первого экрана.
internal actual object SessionStorage {
    private const val PREFS = "coffeely.session"
    private const val KEY_ACCESS = "access"
    private const val KEY_REFRESH = "refresh"

    private var prefs: SharedPreferences? = null

    fun init(context: Context) {
        prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    }

    actual fun save(access: String, refresh: String) {
        prefs?.edit()?.putString(KEY_ACCESS, access)?.putString(KEY_REFRESH, refresh)?.apply()
    }

    actual fun load(): StoredSession? {
        val p = prefs ?: return null
        val access = p.getString(KEY_ACCESS, null) ?: return null
        val refresh = p.getString(KEY_REFRESH, null) ?: return null
        return StoredSession(access, refresh)
    }

    actual fun clear() {
        prefs?.edit()?.clear()?.apply()
    }
}
