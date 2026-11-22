# syntax=docker/dockerfile:1
# Лендинг coffeely: Vite-сборка → nginx (статика, без бэкенда).
FROM node:22-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .

# Ключ Tiles API приходит из CI/build-arg. Он публичный (попадает в бандл),
# защита — ограничение по домену в кабинете Яндекса.
ARG VITE_YANDEX_TILES_KEY
ENV VITE_YANDEX_TILES_KEY=$VITE_YANDEX_TILES_KEY

RUN npm run build

FROM nginx:alpine

RUN rm /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/coffeely.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
