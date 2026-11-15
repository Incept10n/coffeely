// APK публикуется CI-пайплайном в S3 (seaweedfs, https://s3.inceptech.ru).
// Объект перезаписывается каждой сборкой из main, поэтому ссылка стабильная.
const APK_URL = "https://s3.inceptech.ru/coffeely/coffeely.apk";
const CONTACT = "hello@inceptech.ru";

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

export default function App() {
  return (
    <>
      <header className="top">
        <div className="top__inner">
          <a className="logo" href="#top" aria-label="coffeely">
            <span className="logo__mark" aria-hidden="true" />
            coffeely
          </a>
          <nav className="top__nav">
            <a href="#how">Как это работает</a>
            <a href="#shops">Для кофеен</a>
            <a className="top__cta" href="#download">
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
                <a className="button button--ghost" href="#shops">
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
              <p className="pitch__price">от 1 500 ₽ в месяц за точку</p>
              <p className="pitch__note">
                Островок — 1 500 ₽, стандарт — 2 500 ₽, высокий трафик —
                4 900 ₽. Без комиссии с заказа: скидки гостям остаются вашим
                бюджетом.
              </p>
            </div>
            <a
              className="button button--light"
              href={`mailto:${CONTACT}?subject=coffeely%20для%20кофейни`}
            >
              Подключить точку
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
