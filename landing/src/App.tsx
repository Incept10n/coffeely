import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import AdminPanel from "./Admin";
import PointsMap from "./Map";
import type { MapPoint } from "./Map";

// APK публикуется CI-пайплайном в S3 (seaweedfs, https://s3.inceptech.ru).
// Объект перезаписывается каждой сборкой из main, поэтому ссылка стабильная.
const APK_URL = "https://s3.inceptech.ru/coffeely/coffeely.apk";
const CONTACT = "coffeely@inceptech.ru";

// API живёт на том же домене под префиксом /api (nginx срезает префикс).
const API = "/api";

type Tokens = { access: string; refresh: string };
type User = { id: string; email: string; role: string };

const TOKEN_KEY = "coffeely.owner";

function loadTokens(): Tokens | null {
  try {
    const raw = localStorage.getItem(TOKEN_KEY);
    return raw ? (JSON.parse(raw) as Tokens) : null;
  } catch {
    return null;
  }
}
function saveTokens(t: Tokens) {
  localStorage.setItem(TOKEN_KEY, JSON.stringify(t));
}
function clearTokens() {
  localStorage.removeItem(TOKEN_KEY);
}

type ApiOpts = { method?: string; body?: unknown; token?: string | null };

async function api<T>(path: string, opts: ApiOpts = {}): Promise<T> {
  const res = await fetch(API + path, {
    method: opts.method ?? (opts.body ? "POST" : "GET"),
    headers: {
      "Content-Type": "application/json",
      ...(opts.token ? { Authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    let msg = `ошибка ${res.status}`;
    const err = (data as { error?: { message?: unknown } }).error?.message;
    if (typeof err === "string") msg = err;
    else if (err) msg = JSON.stringify(err);
    throw new Error(msg);
  }
  return data as T;
}

function authResponse(tokens: { access_token: string; refresh_token: string }): Tokens {
  return { access: tokens.access_token, refresh: tokens.refresh_token };
}

// Роутинг лендинга: / — витрина, /owner/register — регистрация владельца,
// /owner — личный кабинет (тарифы, точки, бариста), /admin — внутренняя
// админка создателей (сводная аналитика, вход по ADMIN_CRM_SECRET).
// SPA-fallback отдаёт index.html на любой путь (nginx try_files).
export default function App() {
  const path = window.location.pathname;
  if (path.startsWith("/admin")) return <AdminPanel />;
  if (path.startsWith("/owner/register")) return <OwnerRegister />;
  if (path.startsWith("/owner")) return <OwnerCabinet />;
  if (path.startsWith("/map")) return <MapPage />;
  return <Landing />;
}

// Публичная страница карты для WebView в приложении. Точки приходят из
// нативного кода через window.coffeelySetPoints, клик по пину уходит обратно
// в window.coffeelyBridge.onPointClick(id). Своей авторизации у страницы нет:
// список уже загружен приложением.
function MapPage() {
  // Нативный WebView может успеть отдать точки до монтирования React —
  // поэтому читаем их и из window.coffeelyPoints, а не только через callback.
  const [points, setPoints] = useState<MapPoint[]>(() => {
    const w = window as unknown as { coffeelyPoints?: MapPoint[] };
    return Array.isArray(w.coffeelyPoints) ? w.coffeelyPoints : [];
  });
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    const w = window as unknown as {
      coffeelyPoints?: MapPoint[];
      coffeelySetPoints?: (raw: MapPoint[]) => void;
    };
    w.coffeelySetPoints = (raw) => setPoints(Array.isArray(raw) ? raw : []);
    if (Array.isArray(w.coffeelyPoints)) setPoints(w.coffeelyPoints);
    return () => {
      delete w.coffeelySetPoints;
    };
  }, []);

  function onSelect(id: string) {
    setSelected(id);
    const w = window as unknown as {
      coffeelyBridge?: { onPointClick?: (id: string) => void };
    };
    w.coffeelyBridge?.onPointClick?.(id);
  }

  return (
    <PointsMap
      points={points}
      mode="view"
      selectedId={selected}
      onSelect={onSelect}
      height="100%"
      className="map-page"
    />
  );
}

/* ------------------------------- Витрина -------------------------------- */

const steps = [
  {
    n: "01",
    title: "Выберите кофейню",
    text: "В списке видно, открыта ли точка и сколько сейчас ждать: «свободно» или «примерно 5 минут». Без выбора окошек и календарей.",
  },
  {
    n: "02",
    title: "Закажите",
    text: "Точка сама назначает время производства по текущей очереди и сразу показывает, к которому часу кофе будет готов.",
  },
  {
    n: "03",
    title: "Приходите к готовому",
    text: "Статус обновляется сам: принят → готовится → готов, забирайте. Назвать номер заказа на стойке — и всё.",
  },
];

const shopPoints = [
  {
    title: "Дедлайн вместо очереди",
    text: "Бариста видит не «список заказов», а «№412 — к 8:41, делать сейчас». Напиток начинают вовремя, а не когда дойдут руки.",
  },
  {
    title: "Ёмкость точки, а не мечты",
    text: "Лимит заказов на окно настраиваем при подключении точки. В пик очередь не растёт бесконечно: приложение просто назначает более позднее время.",
  },
  {
    title: "Метрика, за которую платят",
    text: "Доля заказов, выданных в дедлайн, считается по точке и по дню. Это отчёт, а не «кажется, стало лучше».",
  },
];

function Landing() {
  return (
    <>
      <header className="top">
        <div className="top__inner">
          <a className="logo" href="/" aria-label="coffeely">
            <span className="logo__mark" aria-hidden="true" />
            coffeely
          </a>
          <nav className="top__nav">
            <a href="/#how">Как это работает</a>
            <a href="/#shops">Для кофеен</a>
            <a href="/owner">Кабинет владельца</a>
            <a className="top__cta" href="/#download">
              Скачать
            </a>
          </nav>
        </div>
      </header>

      <main id="top">
        <section className="hero">
          <div className="hero__inner">
            <div className="hero__text">
              <p className="eyebrow">Предзаказ кофе · пилот</p>
              <h1>
                Кофе готов к тому моменту, когда вы подошли
              </h1>
              <p className="lede">
                Выберите кофейню рядом, закажите и идите. Приложение покажет
                примерное ожидание до заказа и точное время готовности после —
                и обновит статус, пока вы в пути.
              </p>
              <div className="hero__actions">
                <a className="button" href={APK_URL}>
                  Скачать приложение
                  <span className="button__hint">APK · Android 8.0+</span>
                </a>
                <a className="button button--ghost" href="/owner">
                  Создать или войти в кабинет бесплатно
                </a>
              </div>
              <p className="fineprint">
                Пилотная сборка: оплата в кофейне при получении, без карты в
                приложении.
              </p>
            </div>

            <div className="phone" aria-hidden="true">
              <div className="phone__screen">
                <div className="phone__bar">
                  <span>Заказ №128</span>
                  <span className="phone__muted">мои заказы</span>
                </div>
                <p className="phone__label">Готовим ваш кофе</p>
                <p className="phone__time">Будет готов к&nbsp;8:41</p>
                <div className="phone__progress">
                  <span />
                </div>
                <div className="phone__status">готовится</div>
                <div className="phone__card">
                  <div className="phone__row">
                    <span>Капучино ×1</span>
                    <span>250 ₽</span>
                  </div>
                  <div className="phone__row phone__muted">
                    <span>Итого</span>
                    <span>250 ₽ · оплата в кофейне</span>
                  </div>
                </div>
                <div className="phone__card phone__card--ghost">
                  <div className="phone__row">
                    <span>№127 · Американо</span>
                    <span>выдан</span>
                  </div>
                </div>
                <div className="phone__timeline">
                  <span className="dot dot--done" />
                  <span className="line line--done" />
                  <span className="dot dot--active" />
                  <span className="line" />
                  <span className="dot" />
                </div>
                <p className="phone__foot">
                  coffeely на Ленина · ул. Ленина 1
                </p>
              </div>
            </div>
          </div>
        </section>

        <section id="how" className="section">
          <h2>Как это работает</h2>
          <div className="grid grid--3">
            {steps.map((s) => (
              <article className="card" key={s.n}>
                <span className="card__n">{s.n}</span>
                <h3>{s.title}</h3>
                <p>{s.text}</p>
              </article>
            ))}
          </div>
        </section>

        <section id="shops" className="section section--dark">
          <div className="section__head">
            <h2>Для кофеен</h2>
            <p className="section__lede">
              Не «ещё один агрегатор с комиссией», а расписание производства
              точки. Вы платите фиксированно за точку и оставляете себе чек
              целиком.
            </p>
          </div>
          <div className="grid grid--3">
            {shopPoints.map((s) => (
              <article className="card card--dark" key={s.title}>
                <h3>{s.title}</h3>
                <p>{s.text}</p>
              </article>
            ))}
          </div>
          <div className="pitch">
            <div>
              <p className="pitch__price">Личный кабинет для владельцев</p>
              <p className="pitch__note">
                Со всеми предложениями и условиями подключения можно
                ознакомиться в личном кабинете — создайте его бесплатно прямо
                на сайте.
              </p>
            </div>
            <a className="button button--light" href="/owner">
              Создать или войти в кабинет бесплатно
            </a>
          </div>
        </section>

        <section id="download" className="section">
          <div className="download">
            <div>
              <p className="eyebrow">Приложение для гостей</p>
              <h2>Скачать coffeely</h2>
              <p className="lede">
                Android 8.0 и новее. Пока приложение не в Google Play —
                ставится из файла. iOS в разработке.
              </p>
            </div>
            <ol className="install">
              <li>Скачайте APK по кнопке ниже.</li>
              <li>
                В ответ на предупреждение разрешите установку из браузера
                («Разрешить из этого источника»).
              </li>
              <li>Откройте файл и нажмите «Установить».</li>
            </ol>
            <a className="button button--wide" href={APK_URL}>
              Скачать APK
              <span className="button__hint">последняя сборка</span>
            </a>
          </div>
        </section>
      </main>

      <footer className="foot">
        <div className="foot__inner">
          <span className="logo logo--small">
            <span className="logo__mark" aria-hidden="true" />
            coffeely
          </span>
          <span className="foot__muted">
            © {new Date().getFullYear()} Inceptech ·{" "}
            <a href={`mailto:${CONTACT}`}>{CONTACT}</a>
          </span>
        </div>
      </footer>
    </>
  );
}

/* --------------------- Регистрация владельца ---------------------------- */

function OwnerRegister() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (loadTokens()) window.location.replace("/owner");
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const data = await api<{ tokens: { access_token: string; refresh_token: string } }>(
        "/owners/register",
        { body: { email: email.trim(), password } },
      );
      saveTokens(authResponse(data.tokens));
      window.location.href = "/owner";
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
      setBusy(false);
    }
  }

  return (
    <OwnerShell title="Личный кабинет владельца" subtitle="Бесплатно, без карты. В кабинете — предложения, ваши точки и рабочие аккаунты бариста.">
      <form className="owner__form" onSubmit={submit}>
        <label className="field">
          <span>Email</span>
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="owner@кофейня.ру"
          />
        </label>
        <label className="field">
          <span>Пароль (минимум 8 символов)</span>
          <input
            type="password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {error && <p className="owner__error">{error}</p>}
        <button className="button button--wide" disabled={busy}>
          {busy ? "Создаём…" : "Создать кабинет"}
        </button>
      </form>
      <p className="owner__hint">
        Уже есть кабинет?{" "}
        <a href="/owner" className="owner__link">
          Войти
        </a>
      </p>
    </OwnerShell>
  );
}

/* ------------------------- Личный кабинет -------------------------------- */

type Break = { start: string; end: string };

type Point = {
  id: string;
  title: string;
  address: string;
  lat?: number;
  lng?: number;
  capacity_config: {
    window_minutes?: number;
    max_orders_per_window?: number;
    work_start: string;
    work_end: string;
    breaks?: Break[];
    accept_eta_minutes?: number;
  };
  is_active: boolean;
};

type Barista = {
  id: string;
  email: string;
  point_id: string;
  point_title: string;
  created_at: string;
  password?: string;
};

type MenuItem = {
  id: string;
  title: string;
  price_minor: number;
  prep_seconds: number;
  is_available: boolean;
  addons: Addon[];
};

type Addon = {
  id: string;
  menu_item_id: string;
  title: string;
  price_minor: number;
  prep_seconds: number;
  is_available: boolean;
};

const TARIFFS = [
  {
    name: "Единый тариф",
    price: "2 500 ₽/мес",
    note: "за одну точку — без разбивки по типу точки: островок, кофейня, высокий трафик",
  },
];

function OwnerCabinet() {
  const [state, setState] = useState<"loading" | "login" | "ready">("loading");
  const [points, setPoints] = useState<Point[]>([]);
  const [baristas, setBaristas] = useState<Barista[]>([]);
  const [brand, setBrand] = useState("");
  const [profileOk, setProfileOk] = useState(true);
  const [notice, setNotice] = useState<string | null>(null);

  async function refreshData(token: string) {
    const [p, b, profile] = await Promise.all([
      api<{ points: Point[] }>("/admin/points", { token }),
      api<{ baristas: Barista[] }>("/admin/baristas", { token }),
      // Название кофейни — необязательная часть кабинета: если бэкенд старее
      // фронтенда и /admin/profile ещё нет, владелец не должен вылетать из входа.
      api<{ brand_name: string }>("/admin/profile", { token }).catch(() => null),
    ]);
    setPoints(p.points);
    setBaristas(b.baristas);
    setProfileOk(profile !== null);
    if (profile) setBrand(profile.brand_name);
  }

  useEffect(() => {
    void (async () => {
      const t = loadTokens();
      if (!t) {
        setState("login");
        return;
      }
      try {
        const me = await api<User>("/auth/me", { token: t.access });
        if (me.role !== "owner") throw new Error("не владелец");
        await refreshData(t.access);
        setState("ready");
      } catch {
        try {
          const data = await api<{ tokens: { access_token: string; refresh_token: string } }>(
            "/auth/refresh",
            { body: { refresh_token: t.refresh } },
          );
          const fresh = authResponse(data.tokens);
          saveTokens(fresh);
          await refreshData(fresh.access);
          setState("ready");
        } catch {
          clearTokens();
          setState("login");
        }
      }
    })();
  }, []);

  if (state === "loading") {
    return <OwnerShell title="Личный кабинет" subtitle="Загружаем…"><p className="owner__hint">…</p></OwnerShell>;
  }
  if (state === "login") {
    return <OwnerLogin onDone={async () => {
      const t = loadTokens();
      if (t) {
        await refreshData(t.access);
      }
      setState("ready");
    }} />;
  }

  return (
    <OwnerShell
      title={brand || "Личный кабинет"}
      subtitle={
        points.length === 0
          ? "Создайте первую точку — она появится в приложении у гостей."
          : `${brand ? `${brand} · ` : ""}Точек: ${points.length} · бариста: ${baristas.length}`
      }
      onLogout={() => {
        const t = loadTokens();
        if (t) void api("/auth/logout", { body: { refresh_token: t.refresh } });
        clearTokens();
        window.location.href = "/";
      }}
    >
      {profileOk && (
        <section className="owner__block">
          <h3>Кофейня</h3>
          <BrandForm
            brand={brand}
            onSaved={async (name) => {
              setBrand(name);
              setNotice("Название кофейни сохранено.");
            }}
          />
        </section>
      )}

      <section className="owner__block">
        <h3>Тариф</h3>
        <div className="tariffs">
          {TARIFFS.map((t) => (
            <div className="tariff" key={t.name}>
              <p className="tariff__name">{t.name}</p>
              <p className="tariff__price">{t.price}</p>
              <p className="tariff__note">{t.note}</p>
            </div>
          ))}
        </div>
        <p className="owner__hint">
          Без комиссии с заказа: скидки гостям остаются вашим бюджетом.
          Вопросы — <a href={`mailto:${CONTACT}`}>{CONTACT}</a>.
        </p>
      </section>

      <section className="owner__block">
        <h3>Точки</h3>
        {points.length === 0 && <p className="owner__hint">Пока ни одной точки.</p>}
        <div className="list">
          {points.map((p) => (
            <PointRow
              key={p.id}
              point={p}
              onChanged={async (msg) => {
                setNotice(msg);
                const t = loadTokens();
                if (t) await refreshData(t.access);
              }}
            />
          ))}
        </div>
        <AddPointForm
          hasPoints={points.length > 0}
          onCreated={async (msg) => {
            setNotice(msg);
            const t = loadTokens();
            if (t) await refreshData(t.access);
          }}
        />
      </section>

      <section className="owner__block">
        <h3>Бариста</h3>
        {baristas.length === 0 && <p className="owner__hint">Рабочих аккаунтов пока нет.</p>}
        <div className="list">
          {baristas.map((b) => (
            <BaristaRow
              key={b.id}
              barista={b}
              points={points}
              onChanged={async (msg) => {
                setNotice(msg);
                const t = loadTokens();
                if (t) await refreshData(t.access);
              }}
            />
          ))}
        </div>
        <AddBaristaForm
          points={points}
          onCreated={async () => {
            setNotice("Бариста создан — пароль виден в списке выше.");
            const t = loadTokens();
            if (t) await refreshData(t.access);
          }}
        />
      </section>

      {notice && <p className="owner__hint">{notice}</p>}
    </OwnerShell>
  );
}

// Название кофейни — общее для кабинета, не привязано к точке. Точки
// сохраняют свои витринные названия, бренд показывается гостям рядом с ними.
function BrandForm({
  brand,
  onSaved,
}: {
  brand: string;
  onSaved: (name: string) => void | Promise<void>;
}) {
  const [name, setName] = useState(brand);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => setName(brand), [brand]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await api<{ brand_name: string }>("/admin/profile", {
        method: "PATCH",
        body: { brand_name: name.trim() },
        token: loadTokens()?.access,
      });
      setName(res.brand_name);
      await onSaved(res.brand_name);
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="owner__form owner__form--plain" onSubmit={submit}>
      <p className="owner__hint">
        Как называется ваша кофейня. Это главное имя бренда — точки ниже сохраняют
        свои названия (например, «на Ленина»).
      </p>
      <div className="owner__row">
        <label className="field" style={{ flex: "1 1 280px" }}>
          <span>Название кофейни</span>
          <input
            value={name}
            maxLength={80}
            onChange={(e) => setName(e.target.value)}
            placeholder="coffeely"
          />
        </label>
        <button
          className="button"
          style={{ alignSelf: "flex-end", marginBottom: 14 }}
          disabled={busy || name.trim() === brand}
        >
          {busy ? "Сохраняем…" : "Сохранить"}
        </button>
      </div>
      {error && <p className="owner__error">{error}</p>}
    </form>
  );
}

function OwnerLogin({ onDone }: { onDone: () => void | Promise<void> }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const data = await api<{ tokens: { access_token: string; refresh_token: string } }>(
        "/owners/login",
        { body: { email: email.trim(), password } },
      );
      saveTokens(authResponse(data.tokens));
      await onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <OwnerShell title="Вход в кабинет" subtitle="Для владельцев кофейен. Регистрация — бесплатна.">
      <form className="owner__form" onSubmit={submit}>
        <label className="field">
          <span>Email</span>
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label className="field">
          <span>Пароль</span>
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {error && <p className="owner__error">{error}</p>}
        <button className="button button--wide" disabled={busy}>
          {busy ? "Входим…" : "Войти"}
        </button>
      </form>
      <p className="owner__hint">
        Нет кабинета?{" "}
        <a href="/owner/register" className="owner__link">
          Создать бесплатно
        </a>
      </p>
    </OwnerShell>
  );
}

type MenuDraftRow = {
  key: string;
  title: string;
  price: string;
  prep: string;
};

type PointForm = {
  title: string;
  address: string;
  lat: number | null;
  lng: number | null;
  workStart: string;
  workEnd: string;
  acceptEta: string;
  breaks: Break[];
  menu: MenuDraftRow[];
};

let draftKey = 0;
function emptyMenuRow(): MenuDraftRow {
  draftKey += 1;
  return { key: `row-${draftKey}`, title: "", price: "", prep: "180" };
}

function AddPointForm({
  hasPoints,
  onCreated,
}: {
  hasPoints: boolean;
  onCreated: (msg: string) => void | Promise<void>;
}) {
  const [open, setOpen] = useState(!hasPoints);
  const [form, setForm] = useState<PointForm>({
    title: "",
    address: "",
    lat: null,
    lng: null,
    workStart: "08:00",
    workEnd: "20:00",
    acceptEta: "5",
    breaks: [],
    menu: [emptyMenuRow()],
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  type TextKey = Exclude<keyof PointForm, "lat" | "lng" | "breaks" | "menu">;
  const set = (k: TextKey) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  function updateRow(key: string, patch: Partial<MenuDraftRow>) {
    setForm((f) => ({
      ...f,
      menu: f.menu.map((row) => (row.key === key ? { ...row, ...patch } : row)),
    }));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);

    // Пустые строки меню не отправляем: владелец может создать точку и
    // наполнить меню позже через «Товары».
    const filled = form.menu.filter((row) => row.title.trim() || row.price.trim());
    const menu = filled.map((row) => ({
      title: row.title.trim(),
      price_minor: rubToMinor(row.price),
      prep_seconds: Number(row.prep),
    }));
    const badMenu = menu.some(
      (m) => !m.title || !Number.isFinite(m.price_minor) || m.price_minor <= 0 ||
        !Number.isFinite(m.prep_seconds) || m.prep_seconds <= 0,
    );
    if (badMenu) {
      setError("Товар: нужны название, цена в рублях и время приготовления в секундах");
      setBusy(false);
      return;
    }

    try {
      await api("/admin/points", {
        token: loadTokens()?.access,
        body: {
          title: form.title.trim(),
          address: form.address.trim(),
          ...(form.lat != null && form.lng != null
            ? { lat: form.lat, lng: form.lng }
            : {}),
          work_start: form.workStart,
          work_end: form.workEnd,
          accept_eta_minutes: Number(form.acceptEta),
          breaks: cleanBreaks(form.breaks),
          menu,
        },
      });
      setForm((f) => ({
        ...f,
        title: "",
        address: "",
        lat: null,
        lng: null,
        menu: [emptyMenuRow()],
      }));
      setOpen(false);
      await onCreated("Точка создана — видна в приложении у гостей.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <div className="owner__add-point owner__add-point--collapsed">
        <div>
          <p className="owner__add-point-title">Новая точка</p>
          <p className="owner__add-point-note">
            Название, часы работы и меню. Товары и добавки можно добавить сразу.
          </p>
        </div>
        <button type="button" className="button button--accent" onClick={() => setOpen(true)}>
          + Добавить точку
        </button>
      </div>
    );
  }

  return (
    <div className="owner__add-point">
      <div className="owner__add-point-head">
        <p className="owner__add-point-title">Новая точка</p>
        {hasPoints && (
          <button type="button" className="button button--ghost" onClick={() => setOpen(false)}>
            Свернуть
          </button>
        )}
      </div>
      <p className="owner__add-point-note">
        Чем наполните — тем и будет торговать точка. Меню можно добавить сейчас
        или позже кнопкой «Товары».
      </p>
      <form className="owner__form owner__form--plain" onSubmit={submit}>
        <div className="owner__grid">
          <label className="field">
            <span>Название точки</span>
            <input required value={form.title} onChange={set("title")} placeholder="coffeely на Ленина" />
          </label>
          <label className="field">
            <span>Адрес</span>
            <input value={form.address} onChange={set("address")} placeholder="ул. Ленина, 1" />
          </label>
          <label className="field">
            <span>Начало работы</span>
            <input type="time" required value={form.workStart} onChange={set("workStart")} />
          </label>
          <label className="field">
            <span>Конец работы</span>
            <input type="time" required value={form.workEnd} onChange={set("workEnd")} />
          </label>
          <label className="field">
            <span>Бариста возьмёт заказ, мин</span>
            <input
              type="number"
              min={0}
              max={180}
              required
              value={form.acceptEta}
              onChange={set("acceptEta")}
            />
          </label>
        </div>

        <div className="owner__map-picker">
          <p className="owner__menu-editor-title">Где точка на карте</p>
          <p className="owner__hint owner__hint--tight">
            Найдите адрес на карте и поставьте пин. Без координат точка не появится
            на карте у гостей, но останется в списке.
          </p>
          <PointsMap
            mode="picker"
            value={form.lat != null && form.lng != null ? { lat: form.lat, lng: form.lng } : null}
            onChange={(lat, lng) => setForm((f) => ({ ...f, lat, lng }))}
            height={320}
          />
          <p className="owner__hint owner__hint--tight">
            {form.lat != null && form.lng != null
              ? `Координаты: ${form.lat.toFixed(6)}, ${form.lng.toFixed(6)}`
              : "Пин не поставлен — точку не будет видно на карте."}
          </p>
        </div>

        <BreaksEditor
          breaks={form.breaks}
          onChange={(breaks) => setForm((f) => ({ ...f, breaks }))}
        />

        <div className="owner__menu-editor">
          <p className="owner__menu-editor-title">Меню</p>
          <p className="owner__hint owner__hint--tight">
            Название, цена в рублях и сколько секунд готовится напиток.
          </p>
          {form.menu.map((row) => (
            <div className="owner__menu-edit-row" key={row.key}>
              <label className="field">
                <span>Название</span>
                <input
                  value={row.title}
                  onChange={(e) => updateRow(row.key, { title: e.target.value })}
                  placeholder="Капучино"
                />
              </label>
              <label className="field">
                <span>Цена, ₽</span>
                <input
                  inputMode="decimal"
                  value={row.price}
                  onChange={(e) => updateRow(row.key, { price: e.target.value })}
                  placeholder="250"
                />
              </label>
              <label className="field">
                <span>Готовить, сек</span>
                <input
                  type="number"
                  min={1}
                  value={row.prep}
                  onChange={(e) => updateRow(row.key, { prep: e.target.value })}
                />
              </label>
              <button
                type="button"
                className="button button--ghost owner__menu-remove"
                onClick={() =>
                  setForm((f) => ({
                    ...f,
                    menu: f.menu.length > 1
                      ? f.menu.filter((x) => x.key !== row.key)
                      : [emptyMenuRow()],
                  }))
                }
              >
                Убрать
              </button>
            </div>
          ))}
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setForm((f) => ({ ...f, menu: [...f.menu, emptyMenuRow()] }))}
          >
            + Ещё товар
          </button>
        </div>

        {error && <p className="owner__error">{error}</p>}
        <button className="button button--accent button--wide" disabled={busy}>
          {busy ? "Создаём…" : "Создать точку"}
        </button>
      </form>
    </div>
  );
}

// BreaksEditor — перерывы точки (обед, техпауза): в это время гости видят
// «перерыв», а движок ёмкости не выдаёт окна.
function BreaksEditor({
  breaks,
  onChange,
}: {
  breaks: Break[];
  onChange: (breaks: Break[]) => void;
}) {
  return (
    <div className="owner__breaks">
      <p className="owner__menu-editor-title">Перерывы</p>
      <p className="owner__hint owner__hint--tight">
        В перерыв точка не принимает заказы: гости увидят время, когда откроется.
      </p>
      {breaks.length === 0 && <p className="owner__hint owner__hint--tight">Без перерывов.</p>}
      {breaks.map((b, i) => (
        <div className="owner__break-row" key={`break-${i}`}>
          <label className="field">
            <span>Начало</span>
            <input
              type="time"
              value={b.start}
              onChange={(e) =>
                onChange(breaks.map((x, j) => (j === i ? { ...x, start: e.target.value } : x)))
              }
            />
          </label>
          <label className="field">
            <span>Конец</span>
            <input
              type="time"
              value={b.end}
              onChange={(e) =>
                onChange(breaks.map((x, j) => (j === i ? { ...x, end: e.target.value } : x)))
              }
            />
          </label>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => onChange(breaks.filter((_, j) => j !== i))}
          >
            Убрать
          </button>
        </div>
      ))}
      <button
        type="button"
        className="button button--ghost"
        onClick={() => onChange([...breaks, { start: "13:00", end: "13:30" }])}
      >
        + Добавить перерыв
      </button>
    </div>
  );
}

/* --------------------- Редактирование точки и меню ----------------------- */

function PointRow({
  point,
  onChanged,
}: {
  point: Point;
  onChanged: (msg: string) => void | Promise<void>;
}) {
  const [editing, setEditing] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const cap = point.capacity_config ?? {
    work_start: "08:00",
    work_end: "20:00",
    breaks: [],
    accept_eta_minutes: 5,
  };

  return (
    <div className="list__row list__row--stack">
      <div className="owner__row owner__row--between">
        <div>
          <strong>{point.title}</strong>
          {point.address ? <span className="badge"> · {point.address}</span> : null}
          {point.lat == null || point.lng == null ? (
            <span className="badge"> · не на карте</span>
          ) : null}
          {!point.is_active ? <span className="badge"> · выключена</span> : null}
        </div>
        <div className="owner__row owner__row--tight">
          <span className="badge">
            {cap.work_start}–{cap.work_end}
            {(cap.breaks?.length ?? 0) > 0
              ? ` · перерывы ${cap.breaks!.map((b) => `${b.start}–${b.end}`).join(", ")}`
              : ""}
            {` · бариста берёт заказ за ${cap.accept_eta_minutes ?? 5} мин`}
          </span>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setEditing((v) => !v)}
          >
            {editing ? "Отмена" : "Изменить"}
          </button>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setMenuOpen((v) => !v)}
          >
            {menuOpen ? "Скрыть меню" : "Товары и добавки"}
          </button>
        </div>
      </div>
      {editing && (
        <EditPointForm
          point={point}
          onSaved={async () => {
            setEditing(false);
            await onChanged("Точка обновлена.");
          }}
        />
      )}
      {menuOpen && <PointMenu point={point} />}
    </div>
  );
}

function EditPointForm({ point, onSaved }: { point: Point; onSaved: () => void | Promise<void> }) {
  const cap = point.capacity_config ?? {
    work_start: "08:00",
    work_end: "20:00",
    breaks: [],
    accept_eta_minutes: 5,
  };
  const [form, setForm] = useState({
    title: point.title,
    address: point.address,
    lat: point.lat ?? null,
    lng: point.lng ?? null,
    workStart: cap.work_start,
    workEnd: cap.work_end,
    acceptEta: String(cap.accept_eta_minutes ?? 5),
    active: point.is_active,
  });
  const [breaks, setBreaks] = useState<Break[]>(cap.breaks ?? []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  type TextKey = "title" | "address" | "workStart" | "workEnd" | "acceptEta";
  const set = (k: TextKey) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/points/${point.id}`, {
        method: "PATCH",
        token: loadTokens()?.access,
        body: {
          title: form.title.trim(),
          address: form.address.trim(),
          ...(form.lat != null && form.lng != null
            ? { lat: form.lat, lng: form.lng }
            : {}),
          work_start: form.workStart,
          work_end: form.workEnd,
          accept_eta_minutes: Number(form.acceptEta),
          breaks: cleanBreaks(breaks),
          is_active: form.active,
        },
      });
      await onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="owner__form" onSubmit={submit}>
      <div className="owner__grid">
        <label className="field">
          <span>Название точки</span>
          <input required value={form.title} onChange={set("title")} />
        </label>
        <label className="field">
          <span>Адрес</span>
          <input value={form.address} onChange={set("address")} />
        </label>
        <label className="field">
          <span>Начало работы</span>
          <input type="time" required value={form.workStart} onChange={set("workStart")} />
        </label>
        <label className="field">
          <span>Конец работы</span>
          <input type="time" required value={form.workEnd} onChange={set("workEnd")} />
        </label>
        <label className="field">
          <span>Бариста возьмёт заказ, мин</span>
          <input
            type="number"
            min={0}
            max={180}
            required
            value={form.acceptEta}
            onChange={set("acceptEta")}
          />
        </label>
      </div>

      <div className="owner__map-picker">
        <p className="owner__menu-editor-title">Где точка на карте</p>
        <p className="owner__hint owner__hint--tight">
          Найдите адрес на карте и поставьте пин. Без координат точка не появится
          на карте у гостей, но останется в списке.
        </p>
        <PointsMap
          mode="picker"
          value={form.lat != null && form.lng != null ? { lat: form.lat, lng: form.lng } : null}
          onChange={(lat, lng) => setForm((f) => ({ ...f, lat, lng }))}
          height={320}
        />
        <p className="owner__hint owner__hint--tight">
          {form.lat != null && form.lng != null
            ? `Координаты: ${form.lat.toFixed(6)}, ${form.lng.toFixed(6)}`
            : "Пин не поставлен — точку не будет видно на карте."}
        </p>
      </div>

      <BreaksEditor breaks={breaks} onChange={setBreaks} />

      <label className="field field--check">
        <input
          type="checkbox"
          checked={form.active}
          onChange={(e) => setForm((f) => ({ ...f, active: e.target.checked }))}
        />
        <span>Точка активна — видна гостям и принимает заказы</span>
      </label>
      {error && <p className="owner__error">{error}</p>}
      <button className="button" disabled={busy}>
        {busy ? "Сохраняем…" : "Сохранить точку"}
      </button>
    </form>
  );
}

function PointMenu({ point }: { point: Point }) {
  const [items, setItems] = useState<MenuItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [prep, setPrep] = useState("180");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const res = await api<{ items: MenuItem[] }>(`/admin/points/${point.id}/menu`, {
          token: loadTokens()?.access,
        });
        setItems((res.items ?? []).map((it) => ({ ...it, addons: it.addons ?? [] })));
      } catch (err) {
        setError(err instanceof Error ? err.message : "ошибка сети");
      }
    })();
  }, [point.id]);

  async function add(e: FormEvent) {
    e.preventDefault();
    const priceMinor = rubToMinor(price);
    if (
      !title.trim() ||
      !Number.isFinite(priceMinor) ||
      priceMinor <= 0 ||
      !Number.isFinite(Number(prep)) ||
      Number(prep) <= 0
    ) {
      setError("Заполните название, цену в рублях и время приготовления в секундах");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const item = await api<MenuItem>(`/admin/points/${point.id}/menu`, {
        body: { title: title.trim(), price_minor: priceMinor, prep_seconds: Number(prep) },
        token: loadTokens()?.access,
      });
      const created = { ...item, addons: item.addons ?? [] };
      setItems((cur) =>
        cur
          ? [...cur, created].sort((a, b) => a.title.localeCompare(b.title, "ru"))
          : [created],
      );
      setTitle("");
      setPrice("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="owner__menu">
      <p className="owner__hint">
        Товары точки: цена и время приготовления влияют на расчёт времени готовности.
        Добавки настраиваются у каждого товара отдельно — у каких-то их может не быть.
      </p>
      {error && <p className="owner__error">{error}</p>}
      {items === null ? (
        <p className="owner__hint">Загружаем меню…</p>
      ) : items.length === 0 ? (
        <p className="owner__hint">Позиций пока нет.</p>
      ) : (
        <div className="owner__menu-list">
          {items.map((it) => (
            <MenuItemRow
              key={it.id}
              item={it}
              onSaved={(updated) =>
                setItems((cur) => cur && cur.map((x) => (x.id === updated.id ? updated : x)))
              }
            />
          ))}
        </div>
      )}
      <form className="owner__form" onSubmit={add}>
        <div className="owner__menu-row">
          <label className="field">
            <span>Новый товар</span>
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Флэт уайт" />
          </label>
          <label className="field">
            <span>Цена, ₽</span>
            <input inputMode="decimal" value={price} onChange={(e) => setPrice(e.target.value)} placeholder="280" />
          </label>
          <label className="field">
            <span>Готовить, сек</span>
            <input type="number" min={1} value={prep} onChange={(e) => setPrep(e.target.value)} />
          </label>
        </div>
        <button className="button" disabled={busy}>
          {busy ? "Добавляем…" : "Добавить товар"}
        </button>
      </form>
    </div>
  );
}

function MenuItemRow({ item, onSaved }: { item: MenuItem; onSaved: (item: MenuItem) => void }) {
  const [title, setTitle] = useState(item.title);
  const [price, setPrice] = useState(minorToRub(item.price_minor));
  const [prep, setPrep] = useState(String(item.prep_seconds));
  const [addons, setAddons] = useState<Addon[]>(item.addons ?? []);
  const [addonsOpen, setAddonsOpen] = useState(false);
  const [addonTitle, setAddonTitle] = useState("");
  const [addonPrice, setAddonPrice] = useState("");
  const [addonPrep, setAddonPrep] = useState("0");
  const [addonBusy, setAddonBusy] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save(patch: Record<string, unknown>) {
    setBusy(true);
    setError(null);
    try {
      const updated = await api<MenuItem>(`/admin/menu/${item.id}`, {
        method: "PATCH",
        body: patch,
        token: loadTokens()?.access,
      });
      onSaved({ ...updated, addons });
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  async function saveFields(e: FormEvent) {
    e.preventDefault();
    const priceMinor = rubToMinor(price);
    if (
      !title.trim() ||
      !Number.isFinite(priceMinor) ||
      priceMinor <= 0 ||
      !Number.isFinite(Number(prep)) ||
      Number(prep) <= 0
    ) {
      setError("Проверьте название, цену и время приготовления");
      return;
    }
    await save({ title: title.trim(), price_minor: priceMinor, prep_seconds: Number(prep) });
  }

  async function addAddon(e: FormEvent) {
    e.preventDefault();
    const priceMinor = rubToMinor(addonPrice || "0");
    if (
      !addonTitle.trim() ||
      !Number.isFinite(priceMinor) ||
      priceMinor < 0 ||
      !Number.isFinite(Number(addonPrep)) ||
      Number(addonPrep) < 0
    ) {
      setError("Добавка: нужно название, цена в рублях (0 — бесплатно) и время в секундах");
      return;
    }
    setAddonBusy(true);
    setError(null);
    try {
      const addon = await api<Addon>(`/admin/menu/${item.id}/addons`, {
        body: {
          title: addonTitle.trim(),
          price_minor: priceMinor,
          prep_seconds: Number(addonPrep),
        },
        token: loadTokens()?.access,
      });
      setAddons((cur) =>
        [...cur, addon].sort((a, b) => a.title.localeCompare(b.title, "ru")),
      );
      setAddonTitle("");
      setAddonPrice("");
      setAddonPrep("0");
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setAddonBusy(false);
    }
  }

  return (
    <div className="owner__menu-item">
      <form onSubmit={saveFields}>
        <div className="owner__menu-row">
          <label className="field">
            <span>Название</span>
            <input required value={title} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <label className="field">
            <span>Цена, ₽</span>
            <input inputMode="decimal" required value={price} onChange={(e) => setPrice(e.target.value)} />
          </label>
          <label className="field">
            <span>Готовить, сек</span>
            <input type="number" min={1} required value={prep} onChange={(e) => setPrep(e.target.value)} />
          </label>
        </div>
        <div className="owner__row">
          <button className="button" disabled={busy}>
            {busy ? "Сохраняем…" : "Сохранить"}
          </button>
          <button
            type="button"
            className="button button--ghost"
            disabled={busy}
            onClick={() => void save({ is_available: !item.is_available })}
          >
            {item.is_available ? "Скрыть из меню" : "Вернуть в продажу"}
          </button>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setAddonsOpen((v) => !v)}
          >
            {addonsOpen
              ? "Свернуть добавки"
              : `Добавки${addons.length > 0 ? ` · ${addons.length}` : " · нет"}`}
          </button>
          <span className="badge">{item.is_available ? "в продаже" : "скрыт"}</span>
        </div>
      </form>

      {addonsOpen && (
        <div className="owner__item-addons">
          <p className="owner__hint owner__hint--tight">
            Добавки только к «{item.title}»: сироп, молоко, доп. шот. Гость выбирает
            их при заказе, цена прибавляется к чеку.
          </p>
          {addons.length === 0 ? (
            <p className="owner__hint owner__hint--tight">У этого товара добавок нет.</p>
          ) : (
            <div className="owner__addon-list">
              {addons.map((a) => (
                <AddonRow
                  key={a.id}
                  addon={a}
                  onSaved={(updated) =>
                    setAddons((cur) => cur.map((x) => (x.id === updated.id ? updated : x)))
                  }
                  onDeleted={(id) => setAddons((cur) => cur.filter((x) => x.id !== id))}
                />
              ))}
            </div>
          )}
          <form className="owner__menu-row owner__addon-add" onSubmit={addAddon}>
            <label className="field">
              <span>Новая добавка</span>
              <input
                value={addonTitle}
                onChange={(e) => setAddonTitle(e.target.value)}
                placeholder="Сироп карамель"
              />
            </label>
            <label className="field">
              <span>Цена, ₽</span>
              <input
                inputMode="decimal"
                value={addonPrice}
                onChange={(e) => setAddonPrice(e.target.value)}
                placeholder="40"
              />
            </label>
            <label className="field">
              <span>Готовить, сек</span>
              <input
                type="number"
                min={0}
                value={addonPrep}
                onChange={(e) => setAddonPrep(e.target.value)}
              />
            </label>
            <button className="button" disabled={addonBusy}>
              {addonBusy ? "Добавляем…" : "Добавить"}
            </button>
          </form>
        </div>
      )}
      {error && <p className="owner__error">{error}</p>}
    </div>
  );
}

/* ------------------------- Добавки к напиткам ---------------------------- */

function AddonRow({
  addon,
  onSaved,
  onDeleted,
}: {
  addon: Addon;
  onSaved: (addon: Addon) => void;
  onDeleted: (id: string) => void;
}) {
  const [title, setTitle] = useState(addon.title);
  const [price, setPrice] = useState(minorToRub(addon.price_minor));
  const [prep, setPrep] = useState(String(addon.prep_seconds));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save(patch: Record<string, unknown>) {
    setBusy(true);
    setError(null);
    try {
      const updated = await api<Addon>(`/admin/addons/${addon.id}`, {
        method: "PATCH",
        body: patch,
        token: loadTokens()?.access,
      });
      onSaved(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  async function saveFields(e: FormEvent) {
    e.preventDefault();
    const priceMinor = rubToMinor(price || "0");
    if (
      !title.trim() ||
      !Number.isFinite(priceMinor) ||
      priceMinor < 0 ||
      !Number.isFinite(Number(prep)) ||
      Number(prep) < 0
    ) {
      setError("Проверьте название, цену и время приготовления");
      return;
    }
    await save({ title: title.trim(), price_minor: priceMinor, prep_seconds: Number(prep) });
  }

  async function remove() {
    if (!window.confirm(`Убрать добавку «${addon.title}»?`)) return;
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/addons/${addon.id}`, {
        method: "DELETE",
        token: loadTokens()?.access,
      });
      onDeleted(addon.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="owner__addon-item" onSubmit={saveFields}>
      <div className="owner__menu-row">
        <label className="field">
          <span>Добавка</span>
          <input required value={title} onChange={(e) => setTitle(e.target.value)} />
        </label>
        <label className="field">
          <span>Цена, ₽</span>
          <input inputMode="decimal" required value={price} onChange={(e) => setPrice(e.target.value)} />
        </label>
        <label className="field">
          <span>Готовить, сек</span>
          <input type="number" min={0} required value={prep} onChange={(e) => setPrep(e.target.value)} />
        </label>
      </div>
      <div className="owner__row">
        <button className="button" disabled={busy}>
          {busy ? "Сохраняем…" : "Сохранить"}
        </button>
        <button
          type="button"
          className="button button--ghost"
          disabled={busy}
          onClick={() => void save({ is_available: !addon.is_available })}
        >
          {addon.is_available ? "Скрыть" : "Вернуть"}
        </button>
        <button
          type="button"
          className="button button--ghost"
          disabled={busy}
          onClick={() => void remove()}
        >
          Убрать
        </button>
        <span className="badge">{addon.is_available ? "в продаже" : "скрыта"}</span>
      </div>
      {error && <p className="owner__error">{error}</p>}
    </form>
  );
}

/* ------------------ Редактирование аккаунта баристы --------------------- */

function BaristaRow({
  barista,
  points,
  onChanged,
}: {
  barista: Barista;
  points: Point[];
  onChanged: (msg: string) => void | Promise<void>;
}) {
  const [editing, setEditing] = useState(false);

  return (
    <div className="list__row list__row--stack">
      <div className="owner__row owner__row--between">
        <div>
          <strong>{barista.email}</strong>
          <span className="badge"> · {barista.point_title}</span>
        </div>
        <div className="owner__row owner__row--tight">
          <span className="badge">
            пароль:{" "}
            <code className="owner__secret">
              {barista.password || "не сохранён — задайте заново"}
            </code>
          </span>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setEditing((v) => !v)}
          >
            {editing ? "Отмена" : "Изменить"}
          </button>
        </div>
      </div>
      {editing && (
        <EditBaristaForm
          barista={barista}
          points={points}
          onSaved={async () => {
            setEditing(false);
            await onChanged("Аккаунт баристы обновлён.");
          }}
        />
      )}
    </div>
  );
}

function EditBaristaForm({
  barista,
  points,
  onSaved,
}: {
  barista: Barista;
  points: Point[];
  onSaved: () => void | Promise<void>;
}) {
  const [email, setEmail] = useState(barista.email);
  const [password, setPassword] = useState(barista.password ?? "");
  const [pointId, setPointId] = useState(barista.point_id);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (password !== "" && password.length < 8) {
      setError("Пароль короче 8 символов");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const body: { email: string; point_id: string; password?: string } = {
        email: email.trim(),
        point_id: pointId,
      };
      if (password !== "") body.password = password;
      await api<Barista>(`/admin/baristas/${barista.id}`, {
        method: "PATCH",
        body,
        token: loadTokens()?.access,
      });
      await onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="owner__form" onSubmit={submit}>
      <div className="owner__grid">
        <label className="field">
          <span>Email</span>
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label className="field">
          <span>Пароль — виден владельцу и баристе</span>
          <input
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            placeholder="пусто — не менять"
            autoComplete="off"
          />
        </label>
        <label className="field">
          <span>Точка</span>
          <select value={pointId} onChange={(e) => setPointId(e.target.value)}>
            {points.map((p) => (
              <option key={p.id} value={p.id}>
                {p.title}
              </option>
            ))}
          </select>
        </label>
      </div>
      {error && <p className="owner__error">{error}</p>}
      <button className="button" disabled={busy}>
        {busy ? "Сохраняем…" : "Сохранить баристу"}
      </button>
    </form>
  );
}

function rubToMinor(value: string): number {
  return Math.round(Number(value.replace(",", ".").trim()) * 100);
}

// Перерывы с пустыми полями не отправляем: бэкенд справедливо сочтёт их ошибкой.
function cleanBreaks(breaks: Break[]): Break[] {
  return breaks.filter((b) => b.start.trim() && b.end.trim());
}

function minorToRub(minor: number): string {
  return (minor / 100).toFixed(2).replace(/\.00$/, "");
}

function AddBaristaForm({
  points,
  onCreated,
}: {
  points: Point[];
  onCreated: () => void | Promise<void>;
}) {
  const [pointId, setPointId] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!pointId && points.length > 0) setPointId(points[0].id);
  }, [points, pointId]);

  function randomPassword() {
    const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789";
    let out = "";
    for (let i = 0; i < 12; i++) out += alphabet[Math.floor(Math.random() * alphabet.length)];
    setPassword(out);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api<Barista>("/admin/baristas", {
        token: loadTokens()?.access,
        body: { email: email.trim(), password, point_id: pointId },
      });
      setEmail("");
      setPassword("");
      await onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  if (points.length === 0) {
    return <p className="owner__hint">Сначала создайте точку — бариста привязывается к точке.</p>;
  }
  return (
    <form className="owner__form" onSubmit={submit}>
      <div className="owner__grid">
        <label className="field">
          <span>Точка</span>
          <select value={pointId} onChange={(e) => setPointId(e.target.value)}>
            {points.map((p) => (
              <option key={p.id} value={p.id}>
                {p.title}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Email бариста</span>
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label className="field">
          <span>Пароль (мин. 8)</span>
          <input required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
      </div>
      <div className="owner__row">
        <button type="button" className="button button--ghost" onClick={randomPassword}>
          Сгенерировать пароль
        </button>
        <button className="button" disabled={busy || !pointId}>
          {busy ? "Создаём…" : "Создать баристу"}
        </button>
      </div>
      {error && <p className="owner__error">{error}</p>}
    </form>
  );
}

/* --------------------------- Общий каркас -------------------------------- */

function OwnerShell({
  title,
  subtitle,
  children,
  onLogout,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
  onLogout?: () => void;
}) {
  return (
    <>
      <header className="top">
        <div className="top__inner">
          <a className="logo" href="/" aria-label="coffeely">
            <span className="logo__mark" aria-hidden="true" />
            coffeely
          </a>
          <nav className="top__nav">
            {onLogout ? (
              <button type="button" className="top__cta top__cta--button" onClick={onLogout}>
                Выйти
              </button>
            ) : (
              <a className="top__cta" href="/">
                На витрину
              </a>
            )}
          </nav>
        </div>
      </header>
      <main className="owner">
        <div className="owner__head">
          <h1 className="owner__title">{title}</h1>
          <p className="owner__subtitle">{subtitle}</p>
        </div>
        {children}
      </main>
      <footer className="foot">
        <div className="foot__inner">
          <span className="foot__muted">
            © {new Date().getFullYear()} Inceptech ·{" "}
            <a href={`mailto:${CONTACT}`}>{CONTACT}</a>
          </span>
        </div>
      </footer>
    </>
  );
}
