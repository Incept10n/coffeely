package com.coffeely.mobile.screens

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Looper
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import com.coffeely.mobile.api.UserLocation
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

// Спрашиваем разрешение один раз за запуск приложения: отказ не должен
// превращаться в диалог при каждом возврате в «Список точек».
private var askedThisSession = false

@Composable
actual fun rememberUserLocation(): UserLocation? {
    val context = LocalContext.current
    var location by remember { mutableStateOf<UserLocation?>(null) }
    var granted by remember { mutableStateOf(context.hasLocationPermission()) }

    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { result ->
        granted = result.values.any { it }
    }

    // Разрешение спрашиваем один раз за запуск приложения. Отказ не блокирует
    // приложение: список просто останется в порядке сервера.
    LaunchedEffect(Unit) {
        if (!granted && !askedThisSession) {
            askedThisSession = true
            launcher.launch(
                arrayOf(
                    Manifest.permission.ACCESS_FINE_LOCATION,
                    Manifest.permission.ACCESS_COARSE_LOCATION,
                ),
            )
        }
    }

    LaunchedEffect(granted) {
        if (granted) location = context.currentLocation()
    }
    return location
}

private fun Context.hasLocationPermission(): Boolean =
    checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED ||
        checkSelfPermission(Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED

@SuppressLint("MissingPermission")
private suspend fun Context.currentLocation(): UserLocation? {
    val manager = getSystemService(Context.LOCATION_SERVICE) as? LocationManager ?: return null
    val providers = manager.getProviders(true)
    val last = providers.mapNotNull { provider ->
        runCatching { manager.getLastKnownLocation(provider) }.getOrNull()
    }.maxByOrNull { it.time }

    // Свежий last known отдаём сразу, не держим список пустым в ожидании GPS.
    if (last != null && System.currentTimeMillis() - last.time < 10 * 60_000L) {
        return UserLocation(last.latitude, last.longitude)
    }

    val fresh = withTimeoutOrNull(8_000L) {
        suspendCancellableCoroutine { cont ->
            val listener = object : LocationListener {
                override fun onLocationChanged(location: Location) {
                    runCatching { manager.removeUpdates(this) }
                    if (cont.isActive) {
                        cont.resume(UserLocation(location.latitude, location.longitude))
                    }
                }

                override fun onProviderEnabled(provider: String) {}
                override fun onProviderDisabled(provider: String) {}
            }
            providers
                .filter { it == LocationManager.GPS_PROVIDER || it == LocationManager.NETWORK_PROVIDER }
                .forEach { provider ->
                    runCatching {
                        manager.requestLocationUpdates(
                            provider,
                            0L,
                            0f,
                            listener,
                            Looper.getMainLooper(),
                        )
                    }
                }
            cont.invokeOnCancellation { runCatching { manager.removeUpdates(listener) } }
        }
    }
    return fresh ?: last?.let { UserLocation(it.latitude, it.longitude) }
}
