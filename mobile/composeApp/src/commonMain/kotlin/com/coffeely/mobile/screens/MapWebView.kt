package com.coffeely.mobile.screens

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/**
 * Карта точек. Веб-страница `/map` на сайте coffeely рисует Leaflet с тайлами
 * Яндекс Карт; приложение передаёт ей точки и получает клик по пину.
 *
 * Android — WebView, iOS появится вместе с iOS-таргетом.
 */
@Composable
expect fun MapWebView(
    pointsJson: String,
    onPointClick: (String) -> Unit,
    modifier: Modifier = Modifier,
)
