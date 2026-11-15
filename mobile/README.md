# mobile — приложение coffeely (клиент + бариста)

Kotlin Multiplatform + Compose Multiplatform. Android — первый релиз, iOS — в течение полугода.

## Роли в приложении

Один код, два флоу по роли пользователя:

- **client** — точки, меню, выбор 5-минутного окна выдачи, заказ, точное время готовности (realtime).
- **barista** — KDS-экран: очередь заказов с дедлайнами («№412 — к 8:41, делать сейчас»),
  статусы «взял в работу / готов», сводка загрузки окон, фикс промахов.

## Планируемая структура

```
composeApp/        — Compose Multiplatform UI (androidApp + iosApp цели)
core/domain/       — сущности и правила (общие с бэкенд-контрактами)
core/network/      — API-клиент (Ktor), realtime-подписки
core/auth/         — email+пароль, Yandex ID, хранение сессии
feature/client/    — флоу клиента
feature/barista/   — флоу бариста (KDS)
```

## Заметки

- Для сборки нужен JDK 17+ и Android SDK (на этой машине Java пока не установлена).
- Push: FCM, позже RuStore Push.
- Бариста привязывается к точке по коду приглашения.
