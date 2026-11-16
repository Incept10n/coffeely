package com.coffeely.mobile.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Палитра coffeely: крем, латте, кофейный. Заполнены ВСЕ слоты схемы, потому что
// незаполненные берутся из baseline Material 3 — фиолетовые. Именно из-за них
// карточки меню были лавандовыми, диалог регистрации сиреневым, а неактивная
// кнопка «Войти» отдавала фиолетовым оттенком.
private val CoffeeBrown = Color(0xFF6F4E37)
private val CoffeeBrownDeep = Color(0xFF4A3122)
private val Latte = Color(0xFFE7D7C5)
private val CoffeeLight = Color(0xFFA9825C)
private val Cream = Color(0xFFF5EFE6)
private val CreamCard = Color(0xFFFBF5ED)
private val CreamCardAlt = Color(0xFFF0E7DA)
private val Ink = Color(0xFF231B15)
private val InkMuted = Color(0xFF5C4A3D)
private val Outline = Color(0xFF8E7A66)
private val OutlineVariant = Color(0xFFDCCFBE)
private val DarkSurface = Color(0xFF2A211B)
private val DarkBackground = Color(0xFF231B15)
private val DarkInk = Color(0xFFEFE4D9)
private val DarkInkMuted = Color(0xFFD5C4B4)

private val LightColors = lightColorScheme(
    primary = CoffeeBrown,
    onPrimary = Color.White,
    primaryContainer = Latte,
    onPrimaryContainer = CoffeeBrownDeep,
    inversePrimary = Color(0xFFD9BCA3),
    secondary = CoffeeLight,
    onSecondary = Color.White,
    secondaryContainer = Color(0xFFEFE2D3),
    onSecondaryContainer = Color(0xFF3C2B1F),
    tertiary = Color(0xFF8C6A4F),
    onTertiary = Color.White,
    tertiaryContainer = Color(0xFFF0E1D5),
    onTertiaryContainer = Color(0xFF35231A),
    background = Cream,
    onBackground = Ink,
    surface = Cream,
    onSurface = Ink,
    surfaceVariant = CreamCardAlt,
    onSurfaceVariant = InkMuted,
    surfaceTint = CoffeeBrown,
    inverseSurface = Color(0xFF3A2E25),
    inverseOnSurface = Cream,
    error = Color(0xFFB3261E),
    onError = Color.White,
    errorContainer = Color(0xFFF9DEDC),
    onErrorContainer = Color(0xFF410E0B),
    outline = Outline,
    outlineVariant = OutlineVariant,
    scrim = Color.Black,
    surfaceBright = Color(0xFFFFFBF6),
    surfaceDim = Color(0xFFDDD3C6),
    surfaceContainerLowest = Color.White,
    surfaceContainerLow = Color(0xFFFBF6EF),
    surfaceContainer = Cream,
    surfaceContainerHigh = CreamCard,
    surfaceContainerHighest = CreamCardAlt,
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFD9BCA3),
    onPrimary = Color(0xFF3D2A1C),
    primaryContainer = Color(0xFF57402D),
    onPrimaryContainer = Color(0xFFF4DFCB),
    inversePrimary = CoffeeBrown,
    secondary = Color(0xFFC9A889),
    onSecondary = Color(0xFF3D2A1C),
    secondaryContainer = Color(0xFF4A3526),
    onSecondaryContainer = Color(0xFFF2E2D2),
    tertiary = Color(0xFFD8BFA8),
    onTertiary = Color(0xFF3D2A1C),
    tertiaryContainer = Color(0xFF503828),
    onTertiaryContainer = Color(0xFFF2E2D2),
    background = DarkBackground,
    onBackground = DarkInk,
    surface = DarkSurface,
    onSurface = DarkInk,
    surfaceVariant = Color(0xFF45372C),
    onSurfaceVariant = DarkInkMuted,
    surfaceTint = Color(0xFFD9BCA3),
    inverseSurface = DarkInk,
    inverseOnSurface = DarkSurface,
    error = Color(0xFFF2B8B5),
    onError = Color(0xFF601410),
    errorContainer = Color(0xFF8C1D18),
    onErrorContainer = Color(0xFFF9DEDC),
    outline = Color(0xFF9B8874),
    outlineVariant = Color(0xFF4E3F33),
    scrim = Color.Black,
    surfaceBright = Color(0xFF4A3D33),
    surfaceDim = Color(0xFF1B1410),
    surfaceContainerLowest = Color(0xFF181210),
    surfaceContainerLow = Color(0xFF241C17),
    surfaceContainer = DarkSurface,
    surfaceContainerHigh = Color(0xFF33281F),
    surfaceContainerHighest = Color(0xFF3E3128),
)

@Composable
fun CoffeelyTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = if (isSystemInDarkTheme()) DarkColors else LightColors,
        content = content,
    )
}
