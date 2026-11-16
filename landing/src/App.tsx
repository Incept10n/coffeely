import { useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import AdminPanel from "./Admin";

// APK публикуется CI-пайплайном в S3 (seaweedfs, https://s3.inceptech.ru).
// Объект перезаписывается каждой сборкой из main, поэтому ссылка стабильная.
const APK_URL = "https://s3.inceptech.ru/coffeely/coffeely.apk";
const CONTACT = "hello@inceptech.ru";

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
  return <Landing />;
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
                <a className="button button--ghost" href="/owner/register">
                  Я владелец кофейни
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
            <a className="button button--light" href="/owner/register">
              Создать кабинет бесплатно
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
              <span className="button__hint">последняя сборка из main</span>
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

type Point = {
  id: string;
  title: string;
  address: string;
  capacity_config: { window_minutes: number; max_orders_per_window: number; work_start: string; work_end: string };
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
};

const TARIFFS = [
  { name: "Островок", price: "1 500 ₽/мес", note: "одна точка, базовая ёмкость" },
  { name: "Стандарт", price: "2 500 ₽/мес", note: "точка с потоком, приоритет поддержки" },
  { name: "Высокий трафик", price: "4 900 ₽/мес", note: "несколько окон и бариста, расширенная настройка" },
];

function OwnerCabinet() {
  const [state, setState] = useState<"loading" | "login" | "ready">("loading");
  const [points, setPoints] = useState<Point[]>([]);
  const [baristas, setBaristas] = useState<Barista[]>([]);
  const [notice, setNotice] = useState<string | null>(null);

  async function refreshData(token: string) {
    const [p, b] = await Promise.all([
      api<{ points: Point[] }>("/admin/points", { token }),
      api<{ baristas: Barista[] }>("/admin/baristas", { token }),
    ]);
    setPoints(p.points);
    setBaristas(b.baristas);
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
      title="Личный кабинет"
      subtitle={
        points.length === 0
          ? "Создайте первую точку — она появится в приложении у гостей."
          : `Точек: ${points.length} · бариста: ${baristas.length}`
      }
      onLogout={() => {
        const t = loadTokens();
        if (t) void api("/auth/logout", { body: { refresh_token: t.refresh } });
        clearTokens();
        window.location.href = "/";
      }}
    >
      <section className="owner__block">
        <h3>Предложения</h3>
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

type PointForm = {
  title: string;
  address: string;
  windowMinutes: string;
  maxPerWindow: string;
  workStart: string;
  workEnd: string;
  menuText: string;
};

function AddPointForm({ onCreated }: { onCreated: (msg: string) => void | Promise<void> }) {
  const [form, setForm] = useState<PointForm>({
    title: "",
    address: "",
    windowMinutes: "5",
    maxPerWindow: "3",
    workStart: "08:00",
    workEnd: "20:00",
    menuText: "Капучино:25000:180,Латте:27000:150",
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const set = (k: keyof PointForm) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const menu = form.menuText
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .map((s) => {
        const [title, price, prep] = s.split(":").map((x) => x.trim());
        return { title, price_minor: Number(price), prep_seconds: Number(prep) };
      });
    const badMenu = menu.some(
      (m) => !m.title || !Number.isFinite(m.price_minor) || m.price_minor <= 0 ||
        !Number.isFinite(m.prep_seconds) || m.prep_seconds <= 0,
    );
    if (badMenu) {
      setError("Меню: формат «Название:цена_в_копейках:prep_секунд», через запятую");
      setBusy(false);
      return;
    }
    try {
      await api("/admin/points", {
        token: loadTokens()?.access,
        body: {
          title: form.title.trim(),
          address: form.address.trim(),
          window_minutes: Number(form.windowMinutes),
          max_orders_per_window: Number(form.maxPerWindow),
          work_start: form.workStart,
          work_end: form.workEnd,
          menu,
        },
      });
      setForm((f) => ({ ...f, title: "", address: "", menuText: f.menuText }));
      await onCreated("Точка создана — видна в приложении у гостей.");
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
          <input required value={form.title} onChange={set("title")} placeholder="coffeely на Ленина" />
        </label>
        <label className="field">
          <span>Адрес</span>
          <input value={form.address} onChange={set("address")} placeholder="ул. Ленина, 1" />
        </label>
        <label className="field">
          <span>Окно, минут</span>
          <input type="number" min={1} max={60} required value={form.windowMinutes} onChange={set("windowMinutes")} />
        </label>
        <label className="field">
          <span>Заказов на окно</span>
          <input type="number" min={1} max={100} required value={form.maxPerWindow} onChange={set("maxPerWindow")} />
        </label>
        <label className="field">
          <span>Начало работы</span>
          <input required value={form.workStart} onChange={set("workStart")} placeholder="08:00" />
        </label>
        <label className="field">
          <span>Конец работы</span>
          <input required value={form.workEnd} onChange={set("workEnd")} placeholder="20:00" />
        </label>
      </div>
      <label className="field">
        <span>Меню: «Название:цена_в_копейках:prep_секунд», через запятую</span>
        <textarea rows={2} value={form.menuText} onChange={set("menuText")} />
      </label>
      {error && <p className="owner__error">{error}</p>}
      <button className="button" disabled={busy}>
        {busy ? "Создаём…" : "Добавить точку"}
      </button>
    </form>
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
    window_minutes: 5,
    max_orders_per_window: 3,
    work_start: "08:00",
    work_end: "20:00",
  };

  return (
    <div className="list__row list__row--stack">
      <div className="owner__row owner__row--between">
        <div>
          <strong>{point.title}</strong>
          {point.address ? <span className="badge"> · {point.address}</span> : null}
          {!point.is_active ? <span className="badge"> · выключена</span> : null}
        </div>
        <div className="owner__row owner__row--tight">
          <span className="badge">
            окна {cap.window_minutes} мин · до {cap.max_orders_per_window} зак. ·{" "}
            {cap.work_start}–{cap.work_end}
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
            {menuOpen ? "Скрыть товары" : "Товары"}
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
    window_minutes: 5,
    max_orders_per_window: 3,
    work_start: "08:00",
    work_end: "20:00",
  };
  const [form, setForm] = useState({
    title: point.title,
    address: point.address,
    windowMinutes: String(cap.window_minutes),
    maxPerWindow: String(cap.max_orders_per_window),
    workStart: cap.work_start,
    workEnd: cap.work_end,
    active: point.is_active,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
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
          window_minutes: Number(form.windowMinutes),
          max_orders_per_window: Number(form.maxPerWindow),
          work_start: form.workStart,
          work_end: form.workEnd,
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
          <span>Окно, минут</span>
          <input type="number" min={1} max={60} required value={form.windowMinutes} onChange={set("windowMinutes")} />
        </label>
        <label className="field">
          <span>Заказов на окно</span>
          <input type="number" min={1} max={100} required value={form.maxPerWindow} onChange={set("maxPerWindow")} />
        </label>
        <label className="field">
          <span>Начало работы</span>
          <input required value={form.workStart} onChange={set("workStart")} placeholder="08:00" />
        </label>
        <label className="field">
          <span>Конец работы</span>
          <input required value={form.workEnd} onChange={set("workEnd")} placeholder="20:00" />
        </label>
      </div>
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
        setItems(res.items);
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
      setItems((cur) =>
        cur ? [...cur, item].sort((a, b) => a.title.localeCompare(b.title, "ru")) : [item],
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
      <p className="owner__hint">Товары точки: цена и время приготовления влияют на расчёт времени готовности.</p>
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
      onSaved(updated);
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

  return (
    <form className="owner__menu-item" onSubmit={saveFields}>
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
        <span className="badge">{item.is_available ? "в продаже" : "скрыт"}</span>
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
