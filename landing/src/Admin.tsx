import { useCallback, useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";

// Внутренняя админка создателей бизнеса: /admin. Вход — общий секрет
// ADMIN_CRM_SECRET (backend: /api/internal/admin/*), пользователей и ролей нет.
// Секрет живёт в sessionStorage: закрыл вкладку — ввод заново.

const API = "/api";
const SECRET_KEY = "coffeely.admin.secret";
const CONTACT = "hello@inceptech.ru";

class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

type ApiOpts = { method?: string; body?: unknown; headers?: Record<string, string> };

async function api<T>(path: string, opts: ApiOpts = {}): Promise<T> {
  const res = await fetch(API + path, {
    method: opts.method ?? (opts.body ? "POST" : "GET"),
    headers: {
      "Content-Type": "application/json",
      ...(opts.headers ?? {}),
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data: unknown = await res.json().catch(() => ({}));
  if (!res.ok) {
    let msg = `ошибка ${res.status}`;
    const err = (data as { error?: { message?: unknown } }).error?.message;
    if (typeof err === "string") msg = err;
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

/* ------------------------------ Типы API -------------------------------- */

type Totals = {
  owners: number;
  owners_new: number;
  clients: number;
  clients_new: number;
  baristas: number;
  points: number;
  points_active: number;
  orders: number;
  orders_window: number;
  orders_today: number;
  orders_prev: number;
  active_orders: number;
  overdue_now: number;
  revenue_minor: number;
  revenue_window_minor: number;
  revenue_prev_minor: number;
  booked_window_minor: number;
  aov_minor: number;
  orders_growth_pct: number;
  revenue_growth_pct: number;
  given_out_window: number;
  cancelled_window: number;
  cancel_rate_pct: number;
  on_time_window: number;
  missed_window: number;
  on_time_pct: number;
  avg_lateness_seconds: number;
  avg_start_delay_seconds: number;
  avg_make_seconds: number;
  avg_pickup_delay_seconds: number;
  avg_lead_seconds: number;
  repeat_clients: number;
  clients_with_orders: number;
  repeat_rate_pct: number;
};

type Daily = {
  date: string;
  orders: number;
  given_out: number;
  cancelled: number;
  in_flight: number;
  revenue_minor: number;
  new_clients: number;
  new_owners: number;
};

type Funnel = { status: string; count: number; total_minor: number };
type Bucket = { key: number; orders: number };

type Point = {
  id: string;
  title: string;
  address: string;
  owner_email: string;
  is_active: boolean;
  orders_window: number;
  orders_total: number;
  given_out_window: number;
  cancelled_window: number;
  revenue_window_minor: number;
  aov_minor: number;
  on_time: number;
  missed: number;
  on_time_pct: number;
  active_orders: number;
  overdue_now: number;
  baristas: number;
  last_order_at: string | null;
  window_minutes: number;
  max_orders_per_window: number;
  work_start: string;
  work_end: string;
  capacity_per_day: number;
  utilization_pct: number | null;
};

type Owner = {
  email: string;
  created_at: string;
  points: number;
  baristas: number;
  orders_window: number;
  revenue_minor: number;
  last_order_at: string | null;
};

type TopItem = { title: string; qty: number; revenue_minor: number };
type Barista = { email: string; point_title: string; closed: number; avg_lead_seconds: number };

type Analytics = {
  generated_at: string;
  window: { days: number; from: string; to: string };
  totals: Totals;
  daily: Daily[];
  funnel: Funnel[];
  hours: Bucket[];
  weekdays: Bucket[];
  points: Point[];
  owners: Owner[];
  top_items: TopItem[];
  baristas: Barista[];
  activation: {
    clients_no_orders: number;
    owners_no_points: number;
    owners_no_baristas: number;
    points_no_orders: number;
  };
};

/* ------------------------------ Форматы --------------------------------- */

const moneyFmt = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 });
const numFmt = new Intl.NumberFormat("ru-RU");
const pctFmt = new Intl.NumberFormat("ru-RU", {
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});

function money(minor: number): string {
  return `${moneyFmt.format(Math.round(minor / 100))} ₽`;
}

function pct(v: number): string {
  return `${pctFmt.format(v)}%`;
}

// Рост к прошлому окну; при пустой базе сравнения честно показываем «—».
function growth(value: number, prev: number): string {
  if (prev <= 0) return "прошлый период пуст";
  const sign = value > 0 ? "+" : "";
  return `${sign}${pctFmt.format(value)} к прошлому периоду`;
}

function duration(seconds: number): string {
  if (seconds <= 0) return "—";
  if (seconds < 60) return `${Math.round(seconds)} с`;
  if (seconds < 3600) return `${Math.round(seconds / 60)} мин`;
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return m > 0 ? `${h} ч ${m} мин` : `${h} ч`;
}

const STATUS_LABELS: Record<string, string> = {
  paid: "Принят",
  in_production: "Готовится",
  ready: "Готов",
  given_out: "Выдан",
  cancelled: "Отменён",
};

const WEEKDAYS = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];

/* ------------------------------- Экран ---------------------------------- */

export default function AdminPanel() {
  const [secret, setSecret] = useState<string>(() => {
    try {
      return sessionStorage.getItem(SECRET_KEY) ?? "";
    } catch {
      return "";
    }
  });
  const [days, setDays] = useState(30);
  const [data, setData] = useState<Analytics | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  const load = useCallback(async (key: string, period: number) => {
    setBusy(true);
    try {
      const res = await api<Analytics>(`/internal/admin/analytics?days=${period}`, {
        headers: { "X-Admin-Secret": key },
      });
      setData(res);
      setUpdatedAt(new Date());
      setError(null);
    } catch (e) {
      const status = e instanceof ApiError ? e.status : 0;
      if (status === 401 || status === 503) {
        try {
          sessionStorage.removeItem(SECRET_KEY);
        } catch {
          /* приватный режим */
        }
        setSecret("");
        setData(null);
        setError(e instanceof Error ? e.message : "доступ закрыт");
      } else {
        setError(e instanceof Error ? e.message : "ошибка сети");
      }
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (!secret) return;
    void load(secret, days);
    const timer = setInterval(() => void load(secret, days), 30_000);
    return () => clearInterval(timer);
  }, [secret, days, load]);

  async function login(raw: string) {
    const key = raw.trim();
    setBusy(true);
    setError(null);
    try {
      await api("/internal/admin/login", { body: { secret: key } });
      try {
        sessionStorage.setItem(SECRET_KEY, key);
      } catch {
        /* приватный режим: живём до перезагрузки страницы */
      }
      setSecret(key);
    } catch (e) {
      setError(e instanceof Error ? e.message : "ошибка сети");
    } finally {
      setBusy(false);
    }
  }

  function logout() {
    try {
      sessionStorage.removeItem(SECRET_KEY);
    } catch {
      /* ignore */
    }
    setSecret("");
    setData(null);
    setError(null);
  }

  return (
    <>
      <header className="top">
        <div className="top__inner">
          <a className="logo" href="/" aria-label="coffeely">
            <span className="logo__mark" aria-hidden="true" />
            coffeely <span className="admin__badge">admin</span>
          </a>
          <nav className="top__nav">
            <a href="/">Витрина</a>
            {secret ? (
              <button type="button" className="top__cta top__cta--button" onClick={logout}>
                Выйти
              </button>
            ) : null}
          </nav>
        </div>
      </header>

      {!secret ? (
        <AdminLogin busy={busy} error={error} onSubmit={login} />
      ) : data ? (
        <Dashboard
          data={data}
          days={days}
          busy={busy}
          error={error}
          updatedAt={updatedAt}
          onDays={setDays}
          onRefresh={() => void load(secret, days)}
        />
      ) : (
        <main className="admin">
          <p className="owner__hint">
            {error ? `Не удалось загрузить аналитику: ${error}` : "Загружаем аналитику…"}
          </p>
        </main>
      )}

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

/* ------------------------------- Вход ----------------------------------- */

function AdminLogin({
  busy,
  error,
  onSubmit,
}: {
  busy: boolean;
  error: string | null;
  onSubmit: (secret: string) => void | Promise<void>;
}) {
  const [value, setValue] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    void onSubmit(value);
  }

  return (
    <main className="admin admin--login">
      <div className="admin__login">
        <p className="eyebrow">Внутренняя админка</p>
        <h1 className="owner__title">Создатели coffeely</h1>
        <p className="owner__subtitle">
          Сводная аналитика по платформе: владельцы, точки, заказы, деньги и дедлайны.
          Вход по строке доступа из секрета <code>ADMIN_CRM_SECRET</code>.
        </p>
        <form className="owner__form" onSubmit={submit}>
          <label className="field">
            <span>Строка доступа</span>
            <input
              type="password"
              autoFocus
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="секрет админки"
            />
          </label>
          {error && <p className="owner__error">{error}</p>}
          <button className="button button--wide" disabled={busy}>
            {busy ? "Проверяем…" : "Войти"}
          </button>
        </form>
        <p className="owner__hint">
          Ссылка нигде не публикуется: только у команды. Секрет — в переменной
          окружения backend.
        </p>
      </div>
    </main>
  );
}

/* ---------------------------- Дашборд ----------------------------------- */

function Dashboard({
  data,
  days,
  busy,
  error,
  updatedAt,
  onDays,
  onRefresh,
}: {
  data: Analytics;
  days: number;
  busy: boolean;
  error: string | null;
  updatedAt: Date | null;
  onDays: (days: number) => void;
  onRefresh: () => void;
}) {
  const t = data.totals;
  const maxFunnel = Math.max(1, ...data.funnel.map((f) => f.count));

  const exportPoints = () => {
    downloadCsv(`coffeely-points-${data.window.to}.csv`, [
      ["Точка", "Адрес", "Владелец", "Активна", "Заказы (окно)", "Заказы всего", "Выручка (окно), ₽",
        "Средний чек, ₽", "В дедлайне, %", "В работе", "Просрочено", "Загрузка, %", "Бариста", "Последний заказ"],
      ...data.points.map((p) => [
        p.title, p.address, p.owner_email, p.is_active ? "да" : "нет", p.orders_window,
        p.orders_total, Math.round(p.revenue_window_minor / 100), Math.round(p.aov_minor / 100),
        p.on_time_pct, p.active_orders, p.overdue_now, p.utilization_pct ?? "", p.baristas,
        p.last_order_at ?? "",
      ]),
    ]);
  };

  const exportOwners = () => {
    downloadCsv(`coffeely-owners-${data.window.to}.csv`, [
      ["Владелец", "Зарегистрирован", "Точек", "Бариста", "Заказы (окно)", "Выручка всего, ₽", "Последний заказ"],
      ...data.owners.map((o) => [
        o.email, o.created_at.slice(0, 10), o.points, o.baristas, o.orders_window,
        Math.round(o.revenue_minor / 100), o.last_order_at ?? "",
      ]),
    ]);
  };

  return (
    <main className="admin">
      <div className="admin__head">
        <div>
          <p className="eyebrow">Внутренняя админка</p>
          <h1 className="owner__title">Пульт coffeely</h1>
          <p className="owner__subtitle">
            {data.window.from} — {data.window.to} · обновлено{" "}
            {updatedAt ? updatedAt.toLocaleTimeString("ru-RU") : "…"}
            {busy ? " · обновляем…" : ""}
          </p>
        </div>
        <div className="admin__controls">
          <div className="admin__seg">
            {[7, 30, 90].map((d) => (
              <button
                type="button"
                key={d}
                className={d === days ? "admin__seg-btn admin__seg-btn--on" : "admin__seg-btn"}
                onClick={() => onDays(d)}
              >
                {d} дней
              </button>
            ))}
          </div>
          <button type="button" className="button button--ghost" onClick={onRefresh} disabled={busy}>
            Обновить
          </button>
        </div>
      </div>

      {error && <p className="owner__error">{error}</p>}

      <div className="admin__kpis">
        <Kpi
          label="Выручка за период"
          value={money(t.revenue_window_minor)}
          hint={`${growth(t.revenue_growth_pct, t.revenue_prev_minor)} · выдано ${numFmt.format(t.given_out_window)} заказов · всего ${money(t.revenue_minor)}`}
          tone={t.revenue_growth_pct < 0 ? "warn" : undefined}
        />
        <Kpi
          label="Заказы за период"
          value={numFmt.format(t.orders_window)}
          hint={`${growth(t.orders_growth_pct, t.orders_prev)} · сегодня ${numFmt.format(t.orders_today)} · всего ${numFmt.format(t.orders)}`}
          tone={t.orders_growth_pct < 0 ? "warn" : undefined}
        />
        <Kpi
          label="Средний чек"
          value={money(t.aov_minor)}
          hint={`бронь без отмен ${money(t.booked_window_minor)}`}
        />
        <Kpi
          label="В работе сейчас"
          value={numFmt.format(t.active_orders)}
          hint={`просрочено: ${numFmt.format(t.overdue_now)}`}
          tone={t.overdue_now > 0 ? "bad" : "good"}
        />
        <Kpi
          label="Дедлайн"
          value={pct(t.on_time_pct)}
          hint={`вовремя ${t.on_time_window} · промах ${t.missed_window} · +${duration(t.avg_lateness_seconds)}`}
          tone={toneOfPct(t.on_time_pct)}
        />
        <Kpi
          label="Отмены"
          value={pct(t.cancel_rate_pct)}
          hint={`${numFmt.format(t.cancelled_window)} из ${numFmt.format(t.given_out_window + t.cancelled_window)}`}
          tone={t.cancel_rate_pct > 10 ? "warn" : undefined}
        />
        <Kpi
          label="Повторные гости"
          value={pct(t.repeat_rate_pct)}
          hint={`${t.repeat_clients} из ${t.clients_with_orders} с выданным заказом`}
        />
        <Kpi
          label="Клиенты"
          value={numFmt.format(t.clients)}
          hint={`+${t.clients_new} за период`}
        />
        <Kpi
          label="Владельцы"
          value={numFmt.format(t.owners)}
          hint={`+${t.owners_new} за период`}
        />
        <Kpi
          label="Точки"
          value={numFmt.format(t.points)}
          hint={`активных ${t.points_active} · бариста ${t.baristas}`}
        />
      </div>

      <section className="admin__panels">
        <Panel title="Заказы по дням" hint="высота — число заказов в день">
          <Bars
            items={data.daily.map((d, i) => ({
              id: d.date,
              value: d.orders,
              label: barLabel(data.daily.length, i) ? d.date.slice(5) : undefined,
              title: `${d.date}: ${d.orders} заказов, ${money(d.revenue_minor)}, выдано ${d.given_out}`,
              caption: d.date,
              valueLabel: `${numFmt.format(d.orders)} заказов · ${money(d.revenue_minor)} · выдано ${d.given_out} · отменено ${d.cancelled}`,
            }))}
          />
        </Panel>
        <Panel title="Выручка по дням" hint="выданные заказы, ₽">
          <Bars
            items={data.daily.map((d, i) => ({
              id: d.date,
              value: d.revenue_minor,
              label: barLabel(data.daily.length, i) ? d.date.slice(5) : undefined,
              title: `${d.date}: ${money(d.revenue_minor)}`,
              caption: d.date,
              valueLabel: `${money(d.revenue_minor)} · выдано ${d.given_out} заказов`,
            }))}
            accent
          />
        </Panel>
      </section>

      <section className="admin__panels">
        <Panel title="Воронка заказов" hint="текущий статус заказов периода">
          <div className="admin__funnel">
            {data.funnel.map((f) => (
              <div className="admin__funnel-row" key={f.status}>
                <span className="admin__funnel-label">{STATUS_LABELS[f.status] ?? f.status}</span>
                <div className="admin__funnel-track">
                  <div
                    className={`admin__funnel-fill admin__funnel-fill--${f.status}`}
                    style={{ width: `${Math.round((f.count / maxFunnel) * 100)}%` }}
                  />
                </div>
                <span className="admin__funnel-count">
                  {numFmt.format(f.count)} · {money(f.total_minor)}
                </span>
              </div>
            ))}
          </div>
        </Panel>
        <Panel title="Нагрузка по часам" hint="по локальному времени точек">
          <Bars
            items={data.hours.map((h) => ({
              id: h.key,
              value: h.orders,
              label: h.key % 3 === 0 ? String(h.key) : undefined,
              title: `${h.key}:00 — ${h.orders} заказов`,
              caption: `${String(h.key).padStart(2, "0")}:00`,
              valueLabel: `${numFmt.format(h.orders)} заказов`,
            }))}
          />
        </Panel>
      </section>

      <section className="admin__panels">
        <Panel title="Дни недели" hint="когда приходят заказы">
          <Bars
            items={data.weekdays.map((d) => ({
              id: d.key,
              value: d.orders,
              label: WEEKDAYS[d.key - 1],
              title: `${WEEKDAYS[d.key - 1]}: ${d.orders} заказов`,
              caption: WEEKDAYS[d.key - 1],
              valueLabel: `${numFmt.format(d.orders)} заказов`,
            }))}
          />
        </Panel>
        <Panel title="Скорость производства" hint="средние интервалы, период">
          <div className="admin__stages">
            <Stage label="Создан → в работу" value={duration(t.avg_start_delay_seconds)} />
            <Stage label="В работу → готов" value={duration(t.avg_make_seconds)} />
            <Stage label="Готов → выдан" value={duration(t.avg_pickup_delay_seconds)} />
            <Stage label="Создан → выдан" value={duration(t.avg_lead_seconds)} />
          </div>
        </Panel>
      </section>

      <Panel title="Точки" hint="окно: заказы, деньги, дедлайны, загрузка ёмкости">
        <button type="button" className="admin__csv" onClick={exportPoints}>
          Скачать CSV
        </button>
        <div className="admin__scroll">
          <table className="admin__table">
            <thead>
              <tr>
                <th>Точка</th>
                <th>Владелец</th>
                <th className="num">Заказы</th>
                <th className="num">Выручка</th>
                <th className="num">Ср. чек</th>
                <th className="num">Дедлайн</th>
                <th className="num">Сейчас</th>
                <th className="num">Загрузка</th>
                <th>Последний заказ</th>
              </tr>
            </thead>
            <tbody>
              {data.points.length === 0 && (
                <tr>
                  <td colSpan={9} className="muted">
                    Точек пока нет.
                  </td>
                </tr>
              )}
              {data.points.map((p) => (
                <tr key={p.id}>
                  <td>
                    <strong>{p.title}</strong>
                    {p.address ? <span className="admin__sub"> · {p.address}</span> : null}
                    {!p.is_active && <span className="admin__tag admin__tag--off">выкл</span>}
                    <div className="admin__sub">
                      {p.work_start}–{p.work_end} · окно {p.window_minutes} мин · до{" "}
                      {p.max_orders_per_window} зак. · бариста {p.baristas}
                    </div>
                  </td>
                  <td>{p.owner_email || <span className="muted">seed</span>}</td>
                  <td className="num">
                    {numFmt.format(p.orders_window)}
                    <span className="admin__sub"> / {numFmt.format(p.orders_total)}</span>
                  </td>
                  <td className="num">{money(p.revenue_window_minor)}</td>
                  <td className="num">{money(p.aov_minor)}</td>
                  <td className="num">
                    <span className={pctClass(p.on_time_pct)}>{pct(p.on_time_pct)}</span>
                    <span className="admin__sub"> · {p.missed} промах</span>
                  </td>
                  <td className="num">
                    {p.active_orders}
                    {p.overdue_now > 0 && (
                      <span className="admin__tag admin__tag--danger">{p.overdue_now} просм.</span>
                    )}
                  </td>
                  <td className="num">{p.utilization_pct === null ? "—" : pct(p.utilization_pct)}</td>
                  <td>{p.last_order_at ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <Panel title="Владельцы" hint="кто подключился и что приносит">
        <button type="button" className="admin__csv" onClick={exportOwners}>
          Скачать CSV
        </button>
        <div className="admin__scroll">
          <table className="admin__table">
            <thead>
              <tr>
                <th>Владелец</th>
                <th>Регистрация</th>
                <th className="num">Точек</th>
                <th className="num">Бариста</th>
                <th className="num">Заказы (окно)</th>
                <th className="num">Выручка всего</th>
                <th>Последний заказ</th>
              </tr>
            </thead>
            <tbody>
              {data.owners.length === 0 && (
                <tr>
                  <td colSpan={7} className="muted">
                    Владельцев пока нет.
                  </td>
                </tr>
              )}
              {data.owners.map((o) => (
                <tr key={o.email}>
                  <td>{o.email}</td>
                  <td>{o.created_at.slice(0, 10)}</td>
                  <td className="num">{o.points}</td>
                  <td className="num">{o.baristas}</td>
                  <td className="num">{numFmt.format(o.orders_window)}</td>
                  <td className="num">{money(o.revenue_minor)}</td>
                  <td>{o.last_order_at ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <section className="admin__panels">
        <Panel title="Топ позиций" hint="по выручке, без отмен">
          <table className="admin__table admin__table--compact">
            <thead>
              <tr>
                <th>Позиция</th>
                <th className="num">Штук</th>
                <th className="num">Выручка</th>
              </tr>
            </thead>
            <tbody>
              {data.top_items.length === 0 && (
                <tr>
                  <td colSpan={3} className="muted">
                    Заказов ещё не было.
                  </td>
                </tr>
              )}
              {data.top_items.map((it) => (
                <tr key={it.title}>
                  <td>{it.title}</td>
                  <td className="num">{numFmt.format(it.qty)}</td>
                  <td className="num">{money(it.revenue_minor)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>

        <Panel title="Бариста" hint="выдачи за период">
          <table className="admin__table admin__table--compact">
            <thead>
              <tr>
                <th>Аккаунт</th>
                <th>Точка</th>
                <th className="num">Выдал</th>
                <th className="num">Создан → выдан</th>
              </tr>
            </thead>
            <tbody>
              {data.baristas.length === 0 && (
                <tr>
                  <td colSpan={4} className="muted">
                    Выдач пока не было.
                  </td>
                </tr>
              )}
              {data.baristas.map((b) => (
                <tr key={`${b.email}-${b.point_title}`}>
                  <td>{b.email}</td>
                  <td>{b.point_title || "—"}</td>
                  <td className="num">{numFmt.format(b.closed)}</td>
                  <td className="num">{duration(b.avg_lead_seconds)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      </section>

      <Panel title="Активация" hint="где теряются подключённые">
        <div className="admin__activation">
          <Kpi label="Клиенты без заказов" value={numFmt.format(data.activation.clients_no_orders)} hint="зарегистрировались и не заказали" />
          <Kpi label="Владельцы без точек" value={numFmt.format(data.activation.owners_no_points)} hint="создали кабинет, но не точку" />
          <Kpi label="Владельцы без бариста" value={numFmt.format(data.activation.owners_no_baristas)} hint="точка есть, работать некому" />
          <Kpi label="Точки без заказов" value={numFmt.format(data.activation.points_no_orders)} hint="точка есть, гости не пришли" />
        </div>
      </Panel>

      <p className="fineprint">
        Период считается в московском времени, часы и дни недели — в таймзоне точки.
        Данные обновляются каждые 30 секунд.
      </p>
    </main>
  );
}

/* --------------------------- Мелкие блоки -------------------------------- */

function Panel({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return (
    <section className="admin__panel">
      <div className="admin__panel-head">
        <h2 className="admin__panel-title">{title}</h2>
        {hint && <span className="admin__panel-hint">{hint}</span>}
      </div>
      {children}
    </section>
  );
}

function Kpi({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: string;
  hint?: string;
  tone?: "good" | "warn" | "bad";
}) {
  return (
    <div className={`admin__kpi${tone ? ` admin__kpi--${tone}` : ""}`}>
      <p className="admin__kpi-label">{label}</p>
      <p className="admin__kpi-value">{value}</p>
      {hint && <p className="admin__kpi-hint">{hint}</p>}
    </div>
  );
}

type BarItem = {
  id: string | number;
  value: number;
  label?: string;
  title: string;
  caption?: string;
  valueLabel?: string;
};

function Bars({ items, accent }: { items: BarItem[]; accent?: boolean }) {
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(1, ...items.map((i) => i.value));
  const active = hover === null ? null : items[hover];

  return (
    <div className="admin__chart">
      <div className="admin__bars" onMouseLeave={() => setHover(null)}>
        {active && hover !== null && (
          <div
            className="admin__tooltip"
            style={{
              left: `${Math.min(92, Math.max(8, ((hover + 0.5) / items.length) * 100))}%`,
            }}
          >
            {active.caption && <b>{active.caption}</b>}
            {active.valueLabel ?? active.title}
          </div>
        )}
        {items.map((it, i) => (
          <div
            className={`admin__bar-wrap${hover === i ? " admin__bar-wrap--hover" : ""}`}
            key={it.id}
            title={it.title}
            aria-label={it.title}
            onMouseEnter={() => setHover(i)}
            onClick={() => setHover(i)}
          >
            <div
              className={`admin__bar${accent ? " admin__bar--accent" : ""}${it.value === 0 ? " admin__bar--zero" : ""}`}
              style={{ height: `${Math.round((it.value / max) * 100)}%` }}
            />
            {it.label ? <span className="admin__bar-label">{it.label}</span> : null}
          </div>
        ))}
      </div>
      <p className="admin__chart-readout">
        {active
          ? `${active.caption ? `${active.caption}: ` : ""}${active.valueLabel ?? active.title}`
          : "Наведите на столбец — покажу значение"}
      </p>
    </div>
  );
}

function Stage({ label, value }: { label: string; value: string }) {
  return (
    <div className="admin__stage">
      <span className="admin__stage-label">{label}</span>
      <span className="admin__stage-value">{value}</span>
    </div>
  );
}

function barLabel(total: number, index: number): boolean {
  if (total <= 12) return true;
  const step = Math.ceil(total / 10);
  return index % step === 0 || index === total - 1;
}

function toneOfPct(v: number): "good" | "warn" | "bad" | undefined {
  if (v >= 90) return "good";
  if (v >= 75) return "warn";
  if (v > 0) return "bad";
  return undefined;
}

function pctClass(v: number): string {
  if (v >= 90) return "admin__pct--good";
  if (v >= 75) return "admin__pct--warn";
  if (v > 0) return "admin__pct--bad";
  return "muted";
}

/* ------------------------------- CSV ------------------------------------ */

function toCsv(rows: (string | number)[][]): string {
  return rows
    .map((row) =>
      row
        .map((cell) => {
          const s = String(cell ?? "");
          return /[";\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
        })
        .join(";"),
    )
    .join("\r\n");
}

function downloadCsv(name: string, rows: (string | number)[][]) {
  const blob = new Blob([`\uFEFF${toCsv(rows)}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
