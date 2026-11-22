import { useEffect, useRef, useState } from "react";
import * as L from "leaflet";
import "leaflet/dist/leaflet.css";

// Точка на карте. Координаты приходят с бэкенда (points.lat/lng), адрес —
// та самая строка «Адрес», которую владелец ввёл в кабинете.
export type MapPoint = {
  id: string;
  title: string;
  address: string;
  lat: number;
  lng: number;
};

type Props = {
  points?: MapPoint[];
  mode?: "view" | "picker";
  value?: { lat: number; lng: number } | null;
  onChange?: (lat: number, lng: number) => void;
  onSelect?: (id: string) => void;
  selectedId?: string | null;
  height?: number | string;
  className?: string;
};

// Москва — только стартовый вид, пока у точек нет координат.
const DEFAULT_CENTER: L.LatLngTuple = [55.751244, 37.618423];
const YANDEX_MAPS = "https://yandex.ru/maps/";

// Ключ Tiles API публичный: он всё равно уезжает в бандл. Секрет подписи
// сюда класть нельзя — для клиентской карты в кабинете Яндекса должен быть
// включён режим «Опциональная» (без обязательной подписи). Если нужна
// строгая подпись, тайлы придётся проксировать через backend.
function tilesUrl(scale: number): string {
  const key = import.meta.env.VITE_YANDEX_TILES_KEY ?? "";
  return (
    "https://tiles.api-maps.yandex.ru/v1/tiles/" +
    "?x={x}&y={y}&z={z}&lang=ru_RU&l=map&projection=web_mercator" +
    `&scale=${scale}&apikey=${encodeURIComponent(key)}`
  );
}

// Свой пин вместо картинок Leaflet: не зависит от путей к assets и умеет
// подсвечивать выбранную точку.
function pinIcon(selected: boolean): L.DivIcon {
  return L.divIcon({
    className: "",
    html: `<span class="map-pin${selected ? " map-pin--selected" : ""}"></span>`,
    iconSize: [26, 26],
    iconAnchor: [13, 13],
  });
}

export default function PointsMap({
  points = [],
  mode = "view",
  value = null,
  onChange,
  onSelect,
  selectedId = null,
  height = 360,
  className,
}: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const markersRef = useRef<L.LayerGroup | null>(null);
  const tilesRef = useRef<L.TileLayer | null>(null);
  const pickerRef = useRef<L.Marker | null>(null);
  const fittedRef = useRef("");
  const pickerSeenRef = useRef(false);

  // Колбэки меняются на каждом рендере — держим актуальные в ref, чтобы
  // не пересоздавать карту и обработчики.
  const onChangeRef = useRef(onChange);
  const onSelectRef = useRef(onSelect);
  useEffect(() => {
    onChangeRef.current = onChange;
    onSelectRef.current = onSelect;
  }, [onChange, onSelect]);

  const [retry, setRetry] = useState(0);
  const [tileError, setTileError] = useState(false);
  const [locating, setLocating] = useState(false);
  const [locateError, setLocateError] = useState<string | null>(null);
  // Слой «вы здесь»: точка и круг точности живут отдельно от пинов кофеен,
  // чтобы обновление списка точек не стирало геолокацию.
  const hereRef = useRef<L.LayerGroup | null>(null);
  const key = import.meta.env.VITE_YANDEX_TILES_KEY ?? "";

  // Инициализация карты — один раз.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const map = L.map(el, {
      center: DEFAULT_CENTER,
      zoom: 11,
      zoomControl: true,
      attributionControl: true,
    });
    mapRef.current = map;

    // Копирайт Яндекса: логотип и «Открыть в Картах» — отдельным контролом.
    map.attributionControl.setPrefix(false);
    map.attributionControl.addAttribution(
      '© <a href="https://yandex.ru/maps/">Яндекс Карты</a>',
    );

    const attribution = new L.Control({ position: "bottomleft" });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (attribution as any).onAdd = () => {
      const box = document.createElement("div");
      box.className = "map-attrib";
      // Без target=_blank: в WebView приложения ссылку перехватывает
      // shouldOverrideUrlLoading и открывает во внешнем браузере.
      box.innerHTML = `
        <a class="map-attrib__logo" href="${YANDEX_MAPS}">
          <img src="/yandex-logo.svg" alt="Яндекс Карты" />
        </a>
        <a class="map-attrib__open" href="${YANDEX_MAPS}">
          Открыть в Картах
        </a>`;
      const open = box.querySelector<HTMLAnchorElement>(".map-attrib__open");
      const update = () => {
        if (!open) return;
        const c = map.getCenter();
        open.href = `${YANDEX_MAPS}?pt=${c.lng},${c.lat}&z=${map.getZoom()}&l=map`;
      };
      update();
      map.on("moveend zoomend", update);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (attribution as any).__update = update;
      return box;
    };
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (attribution as any).onRemove = () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const update = (attribution as any).__update;
      if (update) map.off("moveend zoomend", update);
    };
    attribution.addTo(map);

    markersRef.current = L.layerGroup().addTo(map);
    hereRef.current = L.layerGroup().addTo(map);

    // WebView иногда отдаёт контейнеру нулевой размер на первом кадре:
    // без invalidateSize карта остаётся пустой.
    const invalidate = () => map.invalidateSize();
    const observer = new ResizeObserver(invalidate);
    observer.observe(el);
    const timer = window.setTimeout(invalidate, 0);

    return () => {
      window.clearTimeout(timer);
      observer.disconnect();
      map.remove();
      mapRef.current = null;
      markersRef.current = null;
      hereRef.current = null;
      tilesRef.current = null;
      pickerRef.current = null;
      fittedRef.current = "";
      pickerSeenRef.current = false;
    };
  }, []);

  // Тайлы: scale=2 на retina, tileSize=256, без detectRetina и zoomOffset —
  // иначе Leaflet просит z+1 и маркеры уезжают относительно подложки.
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (tilesRef.current) {
      map.removeLayer(tilesRef.current);
      tilesRef.current = null;
    }
    if (!key) {
      setTileError(true);
      return;
    }

    setTileError(false);
    let failed = false;
    const layer = L.tileLayer(tilesUrl(window.devicePixelRatio >= 2 ? 2 : 1), {
      tileSize: 256,
      minZoom: 0,
      maxZoom: 20,
    });
    layer.on("tileerror", () => {
      failed = true;
      setTileError(true);
    });
    // «load» приходит и после ошибок: не прячем плашку, если хоть один тайл упал.
    layer.on("load", () => {
      if (!failed) setTileError(false);
    });
    layer.addTo(map);
    tilesRef.current = layer;

    return () => {
      if (tilesRef.current === layer) {
        map.removeLayer(layer);
        tilesRef.current = null;
      }
    };
  }, [key, retry]);

  // Маркеры точек.
  useEffect(() => {
    const map = mapRef.current;
    const markers = markersRef.current;
    if (!map || !markers) return;

    markers.clearLayers();
    const visible = points.filter(
      (p) => Number.isFinite(p.lat) && Number.isFinite(p.lng),
    );
    for (const p of visible) {
      const marker = L.marker([p.lat, p.lng], {
        icon: pinIcon(p.id === selectedId),
        title: p.title,
      });
      marker.on("click", () => onSelectRef.current?.(p.id));
      if (p.address) marker.bindTooltip(p.address, { direction: "top" });
      marker.addTo(markers);
    }

    if (mode !== "view") return;

    const sig = visible.map((p) => `${p.id}:${p.lat}:${p.lng}`).join("|");
    if (sig === fittedRef.current) return;
    fittedRef.current = sig;

    if (visible.length === 0) {
      map.setView(DEFAULT_CENTER, 11);
    } else if (visible.length === 1) {
      map.setView([visible[0].lat, visible[0].lng], 16);
    } else {
      map.fitBounds(
        L.latLngBounds(visible.map((p) => [p.lat, p.lng] as L.LatLngTuple)),
        { padding: [32, 32], maxZoom: 16 },
      );
    }
  }, [points, mode, selectedId]);

  // Пикер владельца: клик по карте ставит пин, пин можно перетащить.
  useEffect(() => {
    const map = mapRef.current;
    if (!map || mode !== "picker") return;

    const onClick = (e: L.LeafletMouseEvent) => {
      onChangeRef.current?.(e.latlng.lat, e.latlng.lng);
    };
    map.on("click", onClick);
    return () => {
      map.off("click", onClick);
    };
  }, [mode]);

  useEffect(() => {
    const map = mapRef.current;
    if (!map || mode !== "picker") return;

    if (!value) {
      if (pickerRef.current) {
        map.removeLayer(pickerRef.current);
        pickerRef.current = null;
      }
      return;
    }

    if (!pickerRef.current) {
      const marker = L.marker([value.lat, value.lng], {
        icon: pinIcon(true),
        draggable: true,
      }).addTo(map);
      marker.on("dragend", () => {
        const pos = marker.getLatLng();
        onChangeRef.current?.(pos.lat, pos.lng);
      });
      pickerRef.current = marker;
    } else {
      pickerRef.current.setLatLng([value.lat, value.lng]);
    }

    if (!pickerSeenRef.current) {
      pickerSeenRef.current = true;
      map.setView([value.lat, value.lng], 16);
    }
  }, [value, mode]);

  // «Где я»: спрашиваем геолокацию браузера/WebView, рисуем точку и круг
  // точности и подводим карту к этому месту. В приложении WebView сам
  // спрашивает разрешение Android (см. MapWebView.android.kt).
  function showMyLocation() {
    const map = mapRef.current;
    if (!map) return;
    if (typeof navigator === "undefined" || !navigator.geolocation) {
      setLocateError("Геолокация на этом устройстве недоступна");
      return;
    }
    setLocateError(null);
    setLocating(true);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        setLocating(false);
        const group = hereRef.current;
        if (!group) return;
        const { latitude, longitude, accuracy } = pos.coords;
        // Минимум 40 м: на точных GPS-данных круг в 5 м визуально исчезает.
        const radius = Math.max(accuracy || 0, 40);
        group.clearLayers();
        const area = L.circle([latitude, longitude], {
          radius,
          color: "#2f6fed",
          weight: 1,
          fillColor: "#2f6fed",
          fillOpacity: 0.14,
          className: "map-here-area",
        }).addTo(group);
        L.circleMarker([latitude, longitude], {
          radius: 7,
          color: "#ffffff",
          weight: 3,
          fillColor: "#2f6fed",
          fillOpacity: 1,
          className: "map-here-dot",
        }).addTo(group);
        map.fitBounds(area.getBounds(), { padding: [36, 36], maxZoom: 17 });
      },
      (err) => {
        setLocating(false);
        setLocateError(
          err.code === err.PERMISSION_DENIED
            ? "Нет доступа к геолокации — разрешите его в настройках"
            : "Не удалось определить местоположение",
        );
      },
      { enableHighAccuracy: true, timeout: 12000, maximumAge: 30000 },
    );
  }

  return (
    <div className={`map-wrap${className ? ` ${className}` : ""}`} style={{ height }}>
      <div ref={containerRef} className="map-canvas" />
      {mode === "view" && (
        <>
          <button
            type="button"
            className={`map-locate${locating ? " map-locate--busy" : ""}`}
            onClick={showMyLocation}
            disabled={locating}
            aria-label="Показать моё местоположение"
          >
            <svg
              className="map-locate__icon"
              viewBox="0 0 24 24"
              width="16"
              height="16"
              aria-hidden="true"
            >
              <path
                fill="currentColor"
                d="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Zm9 3h-2.07A7.01 7.01 0 0 0 13 5.07V3h-2v2.07A7.01 7.01 0 0 0 5.07 11H3v2h2.07A7.01 7.01 0 0 0 11 18.93V21h2v-2.07A7.01 7.01 0 0 0 18.93 13H21v-2Zm-9 5a5 5 0 1 1 0-10 5 5 0 0 1 0 10Z"
              />
            </svg>
            {locating ? "Ищем…" : "Где я"}
          </button>
          {locateError && (
            <div className="map-hint" role="status">
              {locateError}
            </div>
          )}
        </>
      )}
      {tileError && (
        <div className="map-overlay" role="status">
          <p>{key ? "Карта не загрузилась" : "Не задан ключ Яндекс Карт"}</p>
          <button
            type="button"
            className="button button--ghost"
            onClick={() => setRetry((n) => n + 1)}
          >
            Повторить
          </button>
        </div>
      )}
    </div>
  );
}
