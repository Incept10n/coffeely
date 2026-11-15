package com.coffeely.mobile

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Без enableEdgeToEdge: v1 использует обычные инсеты, контент не уходит
        // под системные панели и TopAppBar (edge-to-edge вернём после UI-полировки).
        setContent { App() }
    }
}
