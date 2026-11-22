package com.coffeely.mobile.screens

import androidx.compose.runtime.Composable
import com.coffeely.mobile.api.UserLocation

/**
 * Текущие координаты устройства для сортировки точек по близости.
 *
 * Android спрашивает разрешение при первом показе списка; отказ — не ошибка:
 * список остаётся в серверном порядке, расстояние просто не показывается.
 * Координаты живут только в памяти приложения и на сервер не отправляются.
 */
@Composable
expect fun rememberUserLocation(): UserLocation?
