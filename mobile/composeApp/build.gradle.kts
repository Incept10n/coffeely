import java.util.Properties
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.kotlinMultiplatform)
    alias(libs.plugins.kotlinSerialization)
    alias(libs.plugins.androidApplication)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.composeCompiler)
}

kotlin {
    androidTarget {
        compilerOptions {
            jvmTarget.set(JvmTarget.JVM_17)
        }
    }

    sourceSets {
        commonMain.dependencies {
            implementation(compose.runtime)
            implementation(compose.foundation)
            implementation(compose.material3)
            implementation(compose.ui)

            implementation(libs.kotlinx.coroutines.core)

            implementation(libs.ktor.client.core)
            implementation(libs.ktor.client.content.negotiation)
            implementation(libs.ktor.client.websockets)
            implementation(libs.ktor.serialization.kotlinx.json)
        }
        androidMain.dependencies {
            implementation(libs.androidx.activity.compose)
            implementation(libs.ktor.client.okhttp)
        }
    }
}

android {
    namespace = "com.coffeely.mobile"
    compileSdk = libs.versions.android.compileSdk.get().toInt()

    // Один стабильный ключ подписи на все сборки (CI и локальные). Иначе Android
    // отказывается обновлять установленное приложение: каждая debug-сборка на
    // свежем раннере подписывается новым ключом и получается
    // «app not installed as package conflicts with an existing package».
    // Ключ берётся из mobile/local.properties (см. mobile/README.md) или из env CI.
    val signingProperties = Properties().apply {
        val file = rootProject.file("local.properties")
        if (file.exists()) file.inputStream().use { load(it) }
    }
    fun signingValue(key: String, env: String): String? =
        signingProperties.getProperty(key) ?: System.getenv(env)

    val signingStoreFile = signingValue("coffeely.storeFile", "COFFEELY_STORE_FILE")
    val signingStorePassword = signingValue("coffeely.storePassword", "COFFEELY_STORE_PASSWORD")
    val signingKeyAlias = signingValue("coffeely.keyAlias", "COFFEELY_KEY_ALIAS")
    val signingKeyPassword = signingValue("coffeely.keyPassword", "COFFEELY_KEY_PASSWORD")
    val hasSigningKey = signingStoreFile != null && signingStorePassword != null &&
        signingKeyAlias != null && signingKeyPassword != null

    // Версия — от истории git: versionCode растёт с каждым коммитом, поэтому
    // новая сборка ставится поверх старой как обычное обновление.
    fun git(vararg args: String): String? = try {
        val process = ProcessBuilder("git", *args)
            .directory(rootDir)
            .redirectErrorStream(true)
            .start()
        val output = process.inputStream.bufferedReader().use { it.readText() }.trim()
        if (process.waitFor() == 0 && output.isNotEmpty()) output else null
    } catch (_: Exception) {
        null
    }

    val commitCount = git("rev-list", "--count", "HEAD")?.toIntOrNull() ?: 1
    val shortSha = git("rev-parse", "--short=7", "HEAD")

    defaultConfig {
        applicationId = "com.coffeely.mobile"
        minSdk = libs.versions.android.minSdk.get().toInt()
        targetSdk = libs.versions.android.targetSdk.get().toInt()
        versionCode = commitCount
        versionName = if (shortSha != null) "0.1.0+$shortSha" else "0.1.0"

        // Адрес API на сборке: CI передаёт прод, локально — эмулятор (10.0.2.2).
        val apiBaseUrl = providers.gradleProperty("coffeely.apiBaseUrl")
            .orElse("http://10.0.2.2:8080")
            .get()
        buildConfigField("String", "API_BASE_URL", "\"$apiBaseUrl\"")
    }

    signingConfigs {
        if (hasSigningKey) {
            create("coffeely") {
                storeFile = file(signingStoreFile!!)
                storePassword = signingStorePassword
                keyAlias = signingKeyAlias
                keyPassword = signingKeyPassword
            }
        }
    }

    buildTypes {
        // Обе сборки — одним ключом: локальный debug ставится поверх APK
        // с лендинга и наоборот, без «app not installed».
        getByName("debug") {
            signingConfigs.findByName("coffeely")?.let { signingConfig = it }
        }
        getByName("release") {
            isMinifyEnabled = false
            signingConfigs.findByName("coffeely")?.let { signingConfig = it }
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
