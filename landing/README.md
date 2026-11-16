# landing — сайт coffeely

SPA на https://coffeely.inceptech.ru: витрина, кабинет владельца и внутренняя
админка создателей.

Что отдаёт:

- `/` — витрина: объясняет продукт (гостю и кофейне), кнопка скачивания APK
  `https://s3.inceptech.ru/coffeely/coffeely.apk` (файл публикует CI каждым
  пушем в `main`, ссылка постоянная), CTA для кофеен;
- `/owner/register` и `/owner` — бесплатный кабинет владельца: тарифы, свои
  точки (правка параметров и меню), рабочие аккаунты бариста (пароль виден
  владельцу, аккаунт редактируется) (API `/api/owners/*`, `/api/admin/*`);
- `/admin` — внутренняя админка создателей бизнеса: только поле строки доступа
  (без пояснений на странице), после входа — сводная аналитика по владельцам,
  точкам, заказам, деньгам и дедлайнам (API `/api/internal/admin/*`).
  Ссылка не публикуется.

## Сборка

Vite + React + TypeScript, собирается в nginx-образ (`Dockerfile`):

```sh
npm ci
npm run build      # dist/
docker build -t coffeely-landing .
```

CI (`.github/workflows/ci.yml`, job `landing`) пушит `incept1on/coffeely-landing:front`;
ArgoCD тянет тег с `imagePullPolicy: Always`.

## Структура

```
src/App.tsx     — витрина, регистрация и кабинет владельца
src/Admin.tsx   — внутренняя админка создателей (/admin)
src/main.tsx    — монтирование
src/index.css   — стили и токены (крем/эспрессо), адаптив от 360px
index.html      — title/OG/фавикон (inline SVG, без бинарных ассетов)
```

Дизайн — намеренно без картинок и шрифтов из CDN: страница весит ~80 КБ в gzip
и не зависит от внешних сетей.
