package com.coffeely.mobile

import android.graphics.Color
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.coffeely.mobile.api.SessionStorage

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        // Edge-to-edge: фон приложения уходит под системные панели, поэтому полосы
        // статуса и навигации больше не белеют поверх контента и не «съедают» текст.
        // Прозрачные панели + иконки по теме: тёмные на кремовом, светлые в dark mode.
        // Без этого на Android 15+ (targetSdk 36) панель навигации рисуется системой
        // белой поверх приложения: три кнопки сливаются с фоном, а кнопка «Заказать»
        // уезжает под них.
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.auto(Color.TRANSPARENT, Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.auto(Color.TRANSPARENT, Color.TRANSPARENT),
        )
        super.onCreate(savedInstanceState)
        // Хранилище сессии нужно до первого экрана: гость не логинится заново.
        SessionStorage.init(this)
        setContent { App() }
    }
}
