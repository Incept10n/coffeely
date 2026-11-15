package com.coffeely.mobile.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val CoffeeBrown = Color(0xFF6F4E37)
private val CoffeeLight = Color(0xFFA9825C)
private val Cream = Color(0xFFF5EFE6)
private val DarkSurface = Color(0xFF2A211B)

private val LightColors = lightColorScheme(
    primary = CoffeeBrown,
    secondary = CoffeeLight,
    background = Cream,
    surface = Color.White,
)

private val DarkColors = darkColorScheme(
    primary = CoffeeLight,
    secondary = CoffeeBrown,
    background = DarkSurface,
    surface = DarkSurface,
)

@Composable
fun CoffeelyTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (isSystemInDarkTheme()) DarkColors else LightColors,
        content = content,
    )
}
