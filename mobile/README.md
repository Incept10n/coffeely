# mobile — приложение coffeely (клиент + бариста)

Kotlin Multiplatform + Compose Multiplatform. Android — первый релиз, iOS — в течение полугода.

## Что уже есть (M5)

- Gradle-проект KMP (Kotlin 2.2, Compose Multiplatform 1.9, AGP 8.13) с модулем `composeApp`,
  Android-цель, minSdk 26 / target+compileSdk 36. Debug-apk собирается.
- Реальные флоу против dev-бэкенда:
  - **клиент**: вход/регистрация (M2 API) → список точек → меню + окна выдачи со свободной
    ёмкостью → заказ → карточка заказа с точным временем готовности и поллингом статуса;
  - **бариста**: KDS-очередь с дедлайнами и срочностью («делать сейчас»), кнопки
    «Взять в работу / Готов / Выдан», поллинг очереди каждые 5 с.
- Тема coffeely (Material 3).

## Структура

```
composeApp/src/androidMain/  — MainActivity, манифест
composeApp/src/commonMain/
  api/ApiClient.kt           — Ktor-клиент: auth, точки, окна, заказы, KDS
  api/Models.kt              — DTO API + подписи статусов/срочности
  api/TokenStore.kt          — access-токен сессии (в памяти процесса)
  screens/LoginScreen.kt     — вход и регистрация
  screens/ClientScreens.kt   — точки → меню/окно → заказ → статус
  screens/BaristaScreen.kt   — KDS-очередь баристы
  ui/Theme.kt                — тема coffeely
gradle/libs.versions.toml    — версии всех зависимостей
```

## Как собрать .apk

Нужны: JDK 17+ (на машине есть JDK 27) и Android SDK. На этой машине SDK
установлен в `/opt/android-sdk`. Файл `mobile/local.properties` (в `.gitignore`):

```
sdk.dir=/opt/android-sdk
```

Сборка:

```sh
cd mobile
./gradlew :composeApp:assembleDebug
# результат: mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

Установка на подключённое устройство: `/opt/android-sdk/platform-tools/adb install -r composeApp/build/outputs/apk/debug/composeApp-debug.apk`.

Релизная сборка: `./gradlew :composeApp:assembleRelease` (нужен keystore; `*.keystore`
в `.gitignore`, в репо не попадает).

## Быстрый старт на эмуляторе (пошагово)

### 1. Поднять бэкенд на хосте

```sh
cd backend
make dev-up       # Postgres в docker (если ещё не запущен)
make run          # сервер на :8080
```

Эмулятор видит хост-машину как `10.0.2.2`, поэтому в приложении уже прописан
`http://10.0.2.2:8080` — менять ничего не нужно.

### 2. Собрать apk

```sh
cd mobile
./gradlew :composeApp:assembleDebug
```

Файл появится здесь:

```
mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

(около 11 МБ; `build/` в `.gitignore`, в репо apk не попадает)

### 3. Запустить эмулятор

Обычный запуск с окном:

```sh
source ~/.bashrc            # чтобы PATH видел emulator/adb
emulator -avd my_avd
```

Если падает/тормозит из-за GPU или Wayland (а у тебя software rendering) —
запуск без окна, всё управление через adb:

```sh
emulator -avd my_avd -no-window -no-audio -no-boot-anim -gpu swiftshader_indirect
```

Дождаться загрузки:

```sh
adb wait-for-device
adb shell getprop sys.boot_completed   # должно вернуть 1
```

### 4. Установить и открыть приложение

```sh
cd mobile
adb install -r composeApp/build/outputs/apk/debug/composeApp-debug.apk
adb shell am start -n com.coffeely.mobile/.MainActivity
```

Либо вручную: на главном экране эмулятора найти иконку **coffeely** и тапнуть.

### 5. Тестовые аккаунты (dev-база уже засеяна)

| Роль | Email | Пароль |
|---|---|---|
| клиент | `client@dev.coffeely` | `devpassword1` |
| бариста | `barista@dev.coffeely` | `devpassword1` |
| владелец (CRM) | `owner@dev.coffeely` | `devpassword1` |

Пересоздать/добавить точку с меню: `go run ./cmd/seed ...` (см. `backend/README.md`).

### Полезное для управления эмулятором без окна

```sh
adb exec-out screencap -p > screen.png      # скриншот
adb shell input tap 540 1200                # тап по координатам
adb shell input text "client@dev.coffeely"  # ввод текста
adb shell input keyevent 4                  # «назад» (закрыть клавиатуру)
adb shell uiautomator dump /sdcard/ui.xml && adb shell cat /sdcard/ui.xml  # координаты элементов
adb logcat -d | grep -i coffeely            # логи приложения
```

Если приложение не видит бэкенд: проверь `curl http://localhost:8080/readyz` на хосте
и убедись, что эмулятор запущен (не устройство) — адрес `10.0.2.2` работает только в эмуляторе.
Для реального телефона в `api/ApiClient.kt` подставь IP компьютера в Wi-Fi.

### Адрес dev-бэкенда

- Эмулятор Android: `http://10.0.2.2:8080` (дефолт, см. `api/ApiClient.kt`).
- Реальное устройство: IP компьютера в той же Wi-Fi-сети (бэкенд слушает `:8080`).

## Роли в приложении

Один код, два флоу по роли пользователя:

- **client** — точки, меню, выбор 5-минутного окна выдачи, заказ, точное время готовности (realtime).
- **barista** — KDS-экран: очередь заказов с дедлайнами («№412 — к 8:41, делать сейчас»),
  статусы «взял в работу / готов», сводка загрузки окон, фикс промахов.

## Заметки

- Push: FCM, позже RuStore Push.
- Бариста привязывается к точке по коду приглашения.
- iOS-цель не включаем до отдельного решения (см. `docs/PLAN.md`).
