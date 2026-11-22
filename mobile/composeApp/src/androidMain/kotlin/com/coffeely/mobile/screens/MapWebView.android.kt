package com.coffeely.mobile.screens

import android.annotation.SuppressLint
import android.content.Intent
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import com.coffeely.mobile.BuildConfig

/**
 * Мост из JavaScript в Kotlin. Вызывается на фоновом потоке WebView, поэтому
 * клик пробрасываем в main.
 */
private class MapBridge(private val onClick: (String) -> Unit) {
    @JavascriptInterface
    fun onPointClick(id: String) {
        Handler(Looper.getMainLooper()).post { onClick(id) }
    }
}

// Точки кладём и в window.coffeelyPoints, и в callback: React на странице
// мог ещё не смонтироваться, когда WebView закончил загрузку.
private fun pushPoints(view: WebView, json: String) {
    view.evaluateJavascript(
        "window.coffeelyPoints=$json;" +
            "if (window.coffeelySetPoints) window.coffeelySetPoints($json);",
        null,
    )
}

// Uri.port = -1 для порта по умолчанию: сводим к реальному номеру, чтобы
// https://host и https://host:443 считались одним origin.
private fun effectivePort(uri: Uri): Int = when {
    uri.port >= 0 -> uri.port
    uri.scheme.equals("https", ignoreCase = true) -> 443
    else -> 80
}

@SuppressLint("SetJavaScriptEnabled")
@Composable
actual fun MapWebView(
    pointsJson: String,
    onPointClick: (String) -> Unit,
    modifier: Modifier,
) {
    val latestClick = rememberUpdatedState(onPointClick)
    val bridge = remember {
        MapBridge { id -> latestClick.value(id) }
    }
    val base = remember { BuildConfig.WEB_BASE_URL.trimEnd('/') }
    val url = remember(base) { base + "/map?embedded=1" }
    // JS-мост торчит наружу, поэтому внутри WebView разрешаем только свою
    // страницу. Любой другой адрес уходит во внешний браузер.
    val mapOrigin = remember(base) { Uri.parse(base) }

    AndroidView(
        modifier = modifier,
        factory = { context ->
            WebView(context).apply {
                settings.javaScriptEnabled = true
                settings.domStorageEnabled = true
                settings.loadsImagesAutomatically = true
                settings.allowFileAccess = false
                settings.allowContentAccess = false
                settings.javaScriptCanOpenWindowsAutomatically = false
                addJavascriptInterface(bridge, "coffeelyBridge")

                webViewClient = object : WebViewClient() {
                    override fun onPageFinished(view: WebView, url: String?) {
                        super.onPageFinished(view, url)
                        pushPoints(view, pointsJson)
                    }

                    // Внутри WebView — только страница карты. «Открыть в Картах»
                    // и прочие внешние ссылки открываем в браузере/приложении.
                    override fun shouldOverrideUrlLoading(
                        view: WebView,
                        request: WebResourceRequest,
                    ): Boolean {
                        val uri: Uri = request.url
                        val scheme = uri.scheme.orEmpty().lowercase()
                        if (scheme != "http" && scheme != "https") return true
                        val sameOrigin = uri.host.equals(mapOrigin.host, ignoreCase = true) &&
                            effectivePort(uri) == effectivePort(mapOrigin)
                        if (sameOrigin) return false
                        runCatching {
                            view.context.startActivity(Intent(Intent.ACTION_VIEW, uri))
                        }
                        return true
                    }
                }

                loadUrl(url)
            }
        },
        update = { webView ->
            // Точки могли обновиться (поллинг списка): отдаём их странице.
            pushPoints(webView, pointsJson)
        },
        onRelease = { it.destroy() },
    )
}
