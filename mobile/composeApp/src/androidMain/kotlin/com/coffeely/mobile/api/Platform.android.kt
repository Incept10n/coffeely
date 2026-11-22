package com.coffeely.mobile.api

import com.coffeely.mobile.BuildConfig

// versionCode и versionName приходят из Gradle: versionCode = число коммитов
// в HEAD, versionName = 0.1.0+<sha>. Эти же числа сервер сравнивает в реестре
// сборок (/app/version) и CI публикует в реестр при загрузке APK.
internal actual val platformName: String = "android"
internal actual val currentVersionCode: Int = BuildConfig.VERSION_CODE
internal actual val currentVersionName: String = BuildConfig.VERSION_NAME
