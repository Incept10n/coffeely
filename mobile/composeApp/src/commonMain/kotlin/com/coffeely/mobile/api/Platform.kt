package com.coffeely.mobile.api

// Платформа и версия сборки для реестра обновлений. versionCode — то, что
// сравнивает сервер: у Android он растёт с каждым коммитом, поэтому любая
// новая сборка больше установленной.
internal expect val platformName: String
internal expect val currentVersionCode: Int
internal expect val currentVersionName: String
