package com.coffeely.mobile.ui

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.PullToRefreshState
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.unit.dp

/**
 * Обновление свайпом вниз: пользователь тянет экран, появляется круглая
 * кнопка с иконкой перезагрузки, после отпускания она крутится, пока идёт
 * запрос. Сделано поверх Material 3 PullToRefreshBox — он берёт на себя
 * вложенный скролл и не конфликтует с прокруткой списков.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PullRefresh(
    refreshing: Boolean,
    onRefresh: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable BoxScope.() -> Unit,
) {
    val state = rememberPullToRefreshState()
    PullToRefreshBox(
        isRefreshing = refreshing,
        onRefresh = onRefresh,
        state = state,
        modifier = modifier,
        indicator = { RefreshIndicator(refreshing, state) },
        content = content,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BoxScope.RefreshIndicator(refreshing: Boolean, state: PullToRefreshState) {
    val shown = if (refreshing) 1f else state.distanceFraction.coerceIn(0f, 1f)
    if (shown <= 0.01f) return

    val rotation = if (refreshing) {
        val transition = rememberInfiniteTransition(label = "refresh-spin")
        val angle by transition.animateFloat(
            initialValue = 0f,
            targetValue = 360f,
            animationSpec = infiniteRepeatable(tween(900, easing = LinearEasing)),
            label = "refresh-spin-angle",
        )
        angle
    } else {
        // Пока тянешь — иконка поворачивается вместе с жестом.
        shown * 180f
    }

    Surface(
        modifier = Modifier
            .align(Alignment.TopCenter)
            .padding(top = 10.dp)
            .size(44.dp)
            .graphicsLayer {
                alpha = shown
                translationY = -(1f - shown) * 36.dp.toPx()
                scaleX = 0.82f + 0.18f * shown
                scaleY = 0.82f + 0.18f * shown
            },
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        contentColor = MaterialTheme.colorScheme.primary,
        shadowElevation = 3.dp,
    ) {
        Box(contentAlignment = Alignment.Center) {
            Icon(
                Icons.Filled.Refresh,
                contentDescription = "Обновить",
                modifier = Modifier.size(24.dp).rotate(rotation),
            )
        }
    }
}
