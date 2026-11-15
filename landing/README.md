# landing — сайт coffeely

Статическая страница: https://coffeely.inceptech.ru. Бэкенда нет, JS-запросов к API нет.

Что делает:

- объясняет продукт (гостю и кофейне);
- отдаёт кнопку скачивания APK: `https://s3.inceptech.ru/coffeely/coffeely.apk`
  (файл публикует CI каждым пушем в `main`, ссылка постоянная);
- цену и CTA для кофеен (`mailto:hello@inceptech.ru`).

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
src/App.tsx     — вся страница: hero, «как это работает», блок для кофеен, скачивание
src/index.css   — стили и токены (крем/эспрессо), адаптив от 360px
index.html      — title/OG/фавикон (inline SVG, без бинарных ассетов)
```

Дизайн — намеренно без картинок и шрифтов из CDN: страница весит ~80 КБ в gzip
и не зависит от внешних сетей.
