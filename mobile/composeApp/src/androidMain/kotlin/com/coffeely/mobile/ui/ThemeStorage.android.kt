package com.coffeely.mobile.ui

import android.app.Activity
import android.content.Context
import android.content.SharedPreferences
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat

// Выбранная тема на Android — в отдельном SharedPreferences, чтобы пережить
// перезапуск и не зависеть от сессии (logout тему не сбрасывает).
internal actual object ThemeStorage {
    private const val PREFS = "coffeely.settings"
    private const val KEY_THEME = "theme_mode"

    private var prefs: SharedPreferences? = null

    fun init(context: Context) {
        prefs = context.applicationContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    }

    actual fun load(): ThemeMode = when (prefs?.getString(KEY_THEME, null)) {
        "light" -> ThemeMode.LIGHT
        "dark" -> ThemeMode.DARK
        else -> ThemeMode.SYSTEM
    }

    actual fun save(mode: ThemeMode) {
        val value = when (mode) {
            ThemeMode.SYSTEM -> "system"
            ThemeMode.LIGHT -> "light"
            ThemeMode.DARK -> "dark"
        }
        prefs?.edit()?.putString(KEY_THEME, value)?.apply()
    }
}

// isAppearanceLight* = true означает тёмные иконки (для светлого фона).
@Composable
internal actual fun ApplySystemBarStyle(dark: Boolean) {
    val view = LocalView.current
    if (view.isInEditMode) return
    SideEffect {
        val window = (view.context as? Activity)?.window ?: return@SideEffect
        val controller = WindowCompat.getInsetsController(window, view)
        controller.isAppearanceLightStatusBars = !dark
        controller.isAppearanceLightNavigationBars = !dark
    }
}
