# mobile — приложение coffeely (клиент + бариста)

Kotlin Multiplatform + Compose Multiplatform. Android — первый релиз, iOS — в течение полугода.

## Что уже есть (скелет, веха M5 начата досрочно)

- Gradle-проект KMP (Kotlin 2.2, Compose Multiplatform 1.9, AGP 8.13) с модулем `composeApp`,
  Android-цель, minSdk 26 / target+compileSdk 36.
- Тема coffeely (Material 3), логин-экран (заготовка), два демо-флоу по роли: клиент и
  бариста-KDS (заготовки под M3/M4).
- Ktor-клиент против dev-бэкенда: кнопка «Проверить dev-бэкенд» дергает `/readyz` —
  проверяемая связка мобилка ↔ Go-сервер ↔ Postgres.

## Структура

```
composeApp/src/androidMain/  — MainActivity, манифест
composeApp/src/commonMain/
  api/ApiClient.kt           — Ktor-клиент (базовый URL, health/ready)
  screens/LoginScreen.kt     — вход (UI-заготовка, реальная аутентификация — M2)
  screens/RoleScreens.kt     — флоу клиента и баристы (заготовки)
  ui/Theme.kt                — тема coffeely
gradle/libs.versions.toml    — версии всех зависимостей
```

## Как собрать .apk

Нужны: JDK 17+ (на машине уже есть JDK 27) и Android SDK. SDK на этой машине
пока не установлен. Варианты: поставить через Android Studio, или без него:

```sh
mkdir -p ~/Android/Sdk/cmdline-tools
cd ~/Android/Sdk/cmdline-tools
# скачать commandlinetools-linux с developer.android.com/studio#command-line-tools-only
unzip commandlinetools-linux_*.zip -d latest
latest/bin/sdkmanager "platform-tools" "platforms;android-36" "build-tools;36.0.0"
latest/bin/sdkmanager --licenses   # принять лицензии
```

Затем создать `mobile/local.properties` (не в git):

```
sdk.dir=/home/incept1on/Android/Sdk
```

Сборка:

```sh
cd mobile
./gradlew :composeApp:assembleDebug
# результат: mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

Релизная сборка: `./gradlew :composeApp:assembleRelease` (нужен keystore; `*.keystore`
в `.gitignore`, в репо не попадает).

### Адрес dev-бэкенда

- Эмулятор Android: `http://10.0.2.2:8080` (дефолт, см. `api/ApiClient.kt`).
- Реальное устройство: IP компьютера в той же Wi-Fi-сети.

## Роли в приложении

Один код, два флоу по роли пользователя:

- **client** — точки, меню, выбор 5-минутного окна выдачи, заказ, точное время готовности (realtime).
- **barista** — KDS-экран: очередь заказов с дедлайнами («№412 — к 8:41, делать сейчас»),
  статусы «взял в работу / готов», сводка загрузки окон, фикс промахов.

## Заметки

- Push: FCM, позже RuStore Push.
- Бариста привязывается к точке по коду приглашения.
- iOS-цель не включаем до отдельного решения (см. `docs/PLAN.md`).
