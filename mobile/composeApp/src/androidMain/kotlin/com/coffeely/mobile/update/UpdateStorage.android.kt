package com.coffeely.mobile.update

import android.content.Context
import android.content.SharedPreferences

// Последняя политика обновлений на устройстве. Нужна для одного сценария:
// приложение заблокировано, а сети при старте нет — блокировка не должна
// сниматься перезапуском. init — из MainActivity.
internal actual object UpdateStorage {
    private const val PREFS = "coffeely.updates"
    private const val KEY_INFO = "last_policy"

    private var prefs: SharedPreferences? = null

    fun init(context: Context) {
        prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    }

    actual fun save(json: String) {
        prefs?.edit()?.putString(KEY_INFO, json)?.apply()
    }

    actual fun load(): String? = prefs?.getString(KEY_INFO, null)
}
