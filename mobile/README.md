# mobile — приложение coffeely (клиент + бариста)

Kotlin Multiplatform + Compose Multiplatform. Android — первый релиз, iOS — в течение полугода.

## Что уже есть

- Gradle-проект KMP (Kotlin 2.2, Compose Multiplatform 1.9, AGP 8.13) с модулем `composeApp`,
  Android-цель, minSdk 26 / target+compileSdk 36.
- Реальные флоу против бэкенда:
  - **клиент**: вход/регистрация → список кофеен с текущим ожиданием → меню → заказ
    (окно назначает точка, выбора «окошек» нет) → экран заказа с точным временем
    готовности и live-статусом → «Мои заказы». Кнопка «Регистрация» активна всегда:
    с пустыми полями она открывает окно регистрации (email + пароль);
  - **бариста**: KDS-очередь с дедлайнами и срочностью («делать сейчас»), кнопки
    «Взять в работу / Готов / Выдан», realtime по WebSocket.
- Сессия сохраняется на устройстве (SharedPreferences): при следующем запуске гость
  сразу видит свои заказы, access-токен продлевается по refresh.
- Тема coffeely (Material 3), выбор системная/светлая/тёмная — сохраняется на устройстве.

## Структура

```
composeApp/src/androidMain/  — MainActivity, манифест, SharedPreferences, BuildConfig.API_BASE_URL
composeApp/src/commonMain/
  api/ApiClient.kt           — Ktor-клиент: auth, точки с загрузкой, меню, заказы, KDS, WS
  api/Models.kt              — DTO API + подписи статусов/ожидания
  api/TokenStore.kt          — сессия (память + хранилище платформы)
  screens/LoginScreen.kt     — вход и регистрация
  screens/ClientScreens.kt   — точки → меню/заказ → статус → «мои заказы»
  screens/BaristaScreen.kt   — KDS-очередь баристы
  ui/Theme.kt                — тема coffeely
gradle/libs.versions.toml    — версии всех зависимостей
```

## Как собрать .apk

Нужны **JDK 21** и Android SDK. JDK 27 не подходит: AGP 8.13 падает на
`JdkImageTransform` (`jlink` не находит сигнатуру сборки). Если системный JDK 27 —
поставь JDK 21 (например, в `~/.jdks`) и укажи его Gradle:

```sh
# ~/.gradle/gradle.properties (не в репозитории)
org.gradle.java.home=/home/incept1on/.jdks/jdk-21.0.13+9
```

SDK на этой машине — `/opt/android-sdk`; `mobile/local.properties` (в `.gitignore`):

```
sdk.dir=/opt/android-sdk
```

Сборка:

```sh
cd mobile
./gradlew :composeApp:assembleDebug
# результат: mobile/composeApp/build/outputs/apk/debug/composeApp-debug.apk
```

## Подпись и обновления (важно)

Любая сборка — и CI, и локальная — подписывается **одним** ключом `coffeely`
(RSA 4096, `CN=coffeely, O=Inceptech, OU=mobile, C=RU`,
SHA-256 `66:2E:4A:E3:...:4F:8C:70`, годен до 2054).
Иначе Android на обновлении говорит «app not installed as package conflicts with
an existing package»: подпись не совпала, и приложение надо удалять целиком.
Именно это и происходило, пока CI собирал `assembleDebug` — на каждом раннере
генерировался новый debug-ключ.

Ключ и пароли лежат **вне репозитория**: `~/.android-keys/coffeely/`
(`coffeely-release.jks` + `keystore.env`, права 600). Бэкап этого каталога обязателен:
потеря ключа = потеря возможности обновлять приложение у пользователей.
В CI те же данные — в секретах репозитория `ANDROID_KEYSTORE_BASE64`,
`ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`.

Локально ключ подключается через `mobile/local.properties` (в `.gitignore`):

```
sdk.dir=/opt/android-sdk
coffeely.storeFile=/home/incept1on/.android-keys/coffeely/coffeely-release.jks
coffeely.storePassword=…
coffeely.keyAlias=coffeely
coffeely.keyPassword=…
```

Без этих строк сборка тоже пройдёт, но подпишется debug-ключом машины — такой APK
не обновится поверх APK с лендинга.

`versionCode` считается как число коммитов в `HEAD`, `versionName` — `0.1.0+<sha>`,
поэтому обновление проходит как обычное обновление, без сюрпризов с даунгрейдом.

Переход на новый ключ (или смена ключа вообще) требует **один раз** удалить
приложение на устройстве — дальше обновления идут поверх.

### Адрес API на сборке

Адрес зашивается в `BuildConfig.API_BASE_URL` (Gradle property `coffeely.apiBaseUrl`):

```sh
./gradlew :composeApp:assembleDebug                       # http://10.0.2.2:8080 (эмулятор)
./gradlew :composeApp:assembleDebug \
  -Pcoffeely.apiBaseUrl=https://coffeely.inceptech.ru/api # прод
```

CI (`.github/workflows/ci.yml`, job `apk`) собирает именно прод-сборку и выкладывает её
в S3: `https://s3.inceptech.ru/coffeely/coffeely.apk` (эту ссылку отдаёт лендинг).

Установка на подключённое устройство:
`/opt/android-sdk/platform-tools/adb install -r composeApp/build/outputs/apk/debug/composeApp-debug.apk`.

Релизная сборка с подписью: `./gradlew :composeApp:assembleRelease` (нужен keystore;
`*.keystore` в `.gitignore`). Пилот раздаётся debug-сборкой — ключ добавим перед сторовыми релизами.

## Быстрый старт на эмуляторе (пошагово)

### 1. Поднять бэкенд на хосте

```sh
cd backend
make dev-up       # Postgres в docker (если ещё не запущен)
make run          # сервер на :8080
```

Эмулятор видит хост-машину как `10.0.2.2`, поэтому дефолтный адрес уже правильный.

### 2. Собрать apk

```sh
cd mobile
./gradlew :composeApp:assembleDebug
```

### 3. Запустить эмулятор

Обычный запуск с окном:

```sh
source ~/.bashrc            # чтобы PATH видел emulator/adb
emulator -avd my_avd
```

Если падает/тормозит из-за GPU или Wayland (software rendering) — без окна:

```sh
emulator -avd my_avd -no-window -no-audio -no-boot-anim -gpu swiftshader_indirect
adb wait-for-device
adb shell getprop sys.boot_completed   # должно вернуть 1
```

### 4. Установить и открыть

```sh
adb install -r composeApp/build/outputs/apk/debug/composeApp-debug.apk
adb shell am start -n com.coffeely.mobile/.MainActivity
```

### 5. Тестовые аккаунты (dev-база засеяна через `cmd/seed`)

| Роль | Email | Пароль |
|---|---|---|
| клиент | `client@dev.coffeely` | `devpassword1` |
| бариста | `barista@dev.coffeely` | `devpassword1` |
| владелец (CRM) | `owner@dev.coffeely` | `devpassword1` |

### Полезное для управления эмулятором без окна

```sh
adb exec-out screencap -p > screen.png      # скриншот
adb shell input tap 540 1200                # тап по координатам
adb shell input text "client@dev.coffeely"  # ввод текста
adb shell input keyevent 4                  # «назад» (закрыть клавиатуру)
adb shell uiautomator dump /sdcard/ui.xml && adb shell cat /sdcard/ui.xml  # координаты элементов
adb logcat -d | grep -i coffeely            # логи приложения
```

## Роли в приложении

Один код, два флоу по роли пользователя:

- **client** — точки с ожиданием, меню, заказ без выбора окон, точное время готовности,
  live-статус, «мои заказы».
- **barista** — KDS: очередь с дедлайнами («№412 — к 8:41, делать сейчас»),
  «взял в работу / готов / выдан», сводка промахов.

## Заметки

- Системные панели: приложение работает edge-to-edge (`enableEdgeToEdge` в `MainActivity`),
  цвет под полосами статуса и навигации даёт фон Compose-темы (крем/тёмный кофе).
  Не возвращать белый `windowBackground` и не убирать инсеты: нижние бары получают
  `navigationBarsPadding()`, полноэкранные формы — `safeDrawingPadding()` + `imePadding()`.
  Иначе три кнопки Android снова окажутся белой полосой поверх кнопки «Заказать».
- Палитра: `ui/Theme.kt` заполняет **все** слоты схемы Material 3. Незаполненные слоты
  берутся из baseline (фиолетовые) — так карточки и неактивные кнопки становились
  лавандовыми. Новый цвет добавлять в оба варианта (light/dark) сразу.
- Push: FCM, позже RuStore Push (сейчас статус обновляется по WebSocket + поллингу).
- Бариста привязывается к точке по коду приглашения.
- iOS-цель не включаем до отдельного решения (см. `docs/PLAN.md`).
