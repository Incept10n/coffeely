package com.coffeely.mobile.api

import kotlin.math.PI
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.roundToInt
import kotlin.math.sin
import kotlin.math.sqrt

// Координаты пользователя в том виде, в каком они нужны списку точек.
// Устройство спрашивает разрешение только для сортировки «сначала ближайшие»;
// на сервер координаты не уходят.
data class UserLocation(val lat: Double, val lng: Double)

// Расстояние по большому кругу (формула гаверсинуса), метры.
fun distanceMeters(lat1: Double, lng1: Double, lat2: Double, lng2: Double): Double {
    val earthRadius = 6_371_000.0
    val dLat = (lat2 - lat1) * PI / 180.0
    val dLng = (lng2 - lng1) * PI / 180.0
    val a = sin(dLat / 2) * sin(dLat / 2) +
        cos(lat1 * PI / 180.0) * cos(lat2 * PI / 180.0) *
        sin(dLng / 2) * sin(dLng / 2)
    return earthRadius * 2 * atan2(sqrt(a), sqrt(1 - a))
}

// «350 м», «1,2 км», «12 км» — коротко, как в картах, с запятой по-русски.
fun formatDistance(meters: Double): String {
    if (meters < 950) {
        val m = meters.roundToInt().coerceAtLeast(1)
        return "$m м"
    }
    val tenths = (meters / 100.0).roundToInt()
    return if (tenths % 10 == 0) {
        "${tenths / 10} км"
    } else {
        "${tenths / 10},${tenths % 10} км"
    }
}
