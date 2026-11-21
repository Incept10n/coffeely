package com.coffeely.mobile.api

// Время заказа приходит с сервера в UTC (timestamptz → RFC3339 с Z). Гость
// должен видеть свой часовой пояс (на телефоне), максимум — московский, но
// никогда не UTC. Реализация — платформенная (Android: java.time).
internal expect fun hhmmLocal(iso: String): String
