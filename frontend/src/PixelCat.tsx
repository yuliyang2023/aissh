import type { PetStyle } from './monitoring-api';

export type PetMood = 'watching' | 'resting' | 'warning' | 'critical';

// Integer coordinates and a 3× display scale keep every sprite edge crisp.
export function PixelCat({ mood, style = 'classic' }: { mood: PetMood; style?: PetStyle }) {
  if (style === 'space') return <SpaceCat mood={mood} />;
  return style === 'hacker' ? <HackerCat mood={mood} /> : <ClassicCat mood={mood} />;
}

function AlertEmote({ mood, x, y }: { mood: PetMood; x: number; y: number }) {
  return (
    <g transform={`translate(${x} ${y})`}>
      <g className={`pet-emote pet-alert-emote-${mood}`}>
        <path d="M0 0h2v6H0z M0 8h2v2H0z" />
        {mood === 'critical' && <path d="M4 0h2v6H4z M4 8h2v2H4z" />}
      </g>
    </g>
  );
}

function ClassicCat({ mood }: { mood: PetMood }) {
  const asleep = mood === 'resting';
  const critical = mood === 'critical';
  const alert = mood === 'warning' || mood === 'critical';
  return (
    <svg
      className="pet-sprite"
      viewBox="0 0 56 44"
      shapeRendering="crispEdges"
      role="img"
      aria-label="像素监控小猫"
    >
      <path d="M15 41h28v1H15z M18 42h22v1H18z" fill="#302b3d" opacity=".18" />
      <g className="pet-tail">
        <path d="M38 31h7v-3h3v-7h4v2h2v8h-2v4h-4v3H38z" fill="#493b42" />
        <path d="M39 32h7v-3h3v-7h2v3h2v5h-2v4h-4v3h-8z" fill="#dc955b" />
        <path d="M49 22h2v3h2v3h-4z" fill="#fff0cf" />
        <path d="M40 35h7v-2h4v2h-3v2h-8z" fill="#b66c48" />
      </g>
      <g className="pet-cat-body">
        <path d="M19 27h18v3h3v7h2v4H14v-4h2v-7h3z" fill="#493b42" />
        <path d="M20 28h16v3h3v7h2v2H15v-2h2v-7h3z" fill="#e7a263" />
        <path d="M24 30h9v3h2v6H22v-6h2z" fill="#fff0cf" />
        <path d="M17 33h4v2h-4z M36 32h3v2h-3z" fill="#b66c48" />
        <path d="M16 38h7v2h-7z M33 38h7v2h-7z" fill="#fff0cf" />
        <path d="M22 37h1v3h-1z M33 37h1v3h-1z" fill="#b66c48" />
        <path d="M17 28h22v3H17z M34 31h4v5h-4z" fill="#38675e" />
        <path d="M18 28h20v2H18z M35 31h2v3h-2z" fill="#8fc4a3" />
        <path className="pet-badge" d="M27 30h3v3h-3z" />
        <g className="pet-head">
          <path
            d="M12 5h5v2h3v3h3v1h12v-1h3V7h3V5h5v18h-2v4h-4v2H17v-2h-4v-3h-2V13h1z"
            fill="#493b42"
          />
          <path
            d="M13 6h3v2h3v3h3v1h14v-1h3V8h3V6h3v17h-2v3h-4v2H18v-2h-4v-3h-2v-9h1z"
            fill="#edb477"
          />
          <path d="M14 8h2v2h2v4h-4z M42 8h2v6h-4v-4h2z" fill="#bd745e" />
          <path d="M15 10h1v2h2v2h-3z M42 10h1v4h-3v-2h2z" fill="#f4b6a0" />
          <path d="M22 12h3v4h-3z M28 12h3v5h-3z M34 12h2v4h-2z" fill="#c6804f" />
          <path d="M13 17h3v2h-3z M41 17h4v2h-4z" fill="#d48c55" />
          <path d="M23 20h12v2h5v4h-2v1H19v-1h-3v-4h7z" fill="#fff0cf" />
          <g fill="#493b42">
            {asleep ? (
              <path d="M19 20h5v1h-5z M34 20h5v1h-5z" />
            ) : (
              <g className="pet-eyes">
                <path d="M20 18h3v4h-3z M35 18h3v4h-3z" />
                <path d="M20 18h1v1h-1z M35 18h1v1h-1z" fill="#fff8e8" />
              </g>
            )}
            {alert && (
              <path
                d={
                  critical
                    ? 'M19 15h2v1h2v1h-2v-1h-2z M35 16h2v-1h2v1h-2v1h-2z'
                    : 'M19 16h4v1h-4z M35 16h4v1h-4z'
                }
              />
            )}
            <path
              d={
                critical
                  ? 'M27 24h5v3h-5z'
                  : alert
                    ? 'M28 24h3v2h-3z'
                    : 'M25 24h1v1h3v-1h1v1h3v-1h1v2h-9z'
              }
            />
          </g>
          <path d="M28 22h3v1h-3z M29 23h1v1h-1z" fill="#ba736b" />
          <path d="M17 23h4v1h-4z M37 23h4v1h-4z" fill="#e8a090" />
          <path d="M10 21h5v1h-5z M11 24h4v1h-4z M43 21h5v1h-5z M43 24h4v1h-4z" fill="#806052" />
        </g>
      </g>
      {asleep ? (
        <path
          className="pet-emote"
          d="M47 8h5v1h-1v1h-1v1h-1v1h3v1h-5v-1h1v-1h1v-1h1V9h-3z"
          fill="#a7b4c9"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={48} y={7} />
      ) : (
        <path className="pet-emote" d="M49 8h1v2h2v1h-2v2h-1v-2h-2v-1h2z" fill="#8fc4a3" />
      )}
    </svg>
  );
}

function HackerCat({ mood }: { mood: PetMood }) {
  const asleep = mood === 'resting';
  const critical = mood === 'critical';
  const alert = mood === 'warning' || mood === 'critical';
  return (
    <svg
      className="pet-sprite pet-hacker-sprite"
      viewBox="0 0 56 44"
      shapeRendering="crispEdges"
      role="img"
      aria-label="黑客猫猫"
    >
      <path d="M13 41h32v1H13z M16 42h26v1H16z" fill="#081a20" opacity=".25" />
      <g className="pet-tail">
        <path d="M39 32h6v-3h3v-7h4v2h2v8h-2v4h-5v3h-8z" fill="#101b24" />
        <path d="M40 33h6v-3h3v-7h2v3h2v5h-2v4h-5v3h-6z" fill="#344c56" />
        <path d="M49 23h2v3h2v3h-4z M42 35h4v2h-4z" fill="#6df7b0" />
      </g>
      <g className="pet-cat-body">
        <path d="M19 26h18v3h4v7h2v5H13v-5h2v-7h4z" fill="#101b24" />
        <path d="M20 28h16v2h4v9H16v-9h4z" fill="#263c47" />
        <path d="M20 29h2v5h-2z M34 29h2v5h-2z" fill="#6df7b0" />
        <path d="M15 38h7v2h-7z M34 38h7v2h-7z" fill="#45616a" />
        <g className="pet-head">
          <path
            d="M12 4h5v2h3v3h3v1h12V9h3V6h3V4h5v18h-2v4h-4v3H17v-2h-4v-4h-2V12h1z"
            fill="#101b24"
          />
          <path
            d="M13 5h3v2h3v3h3v1h14v-1h3V7h3V5h3v17h-2v4h-4v2H18v-2h-4v-4h-2v-9h1z"
            fill="#344c56"
          />
          <path d="M14 7h2v3h3v4h-5z M42 7h2v7h-5v-4h3z" fill="#6df7b0" />
          <path d="M15 10h1v2h2v2h-3z M42 10h1v4h-3v-2h2z" fill="#2e9b80" />
          <path d="M19 12h19v2h3v2h2v7h-3v3H18v-3h-3v-7h2v-2h2z" fill="#6df7b0" />
          <path d="M20 13h17v2h3v2h2v5h-3v3H19v-3h-3v-5h2v-2h2z" fill="#101b24" />
          <path d="M20 15h17v1h3v6h-3v2H20v-2h-3v-5h3z" fill="#253842" />
          {asleep ? (
            <path d="M20 20h5v1h-5z M33 20h5v1h-5z" fill="#76bca7" />
          ) : (
            <g className="pet-eyes">
              <path className="pet-badge" d="M20 18h5v4h-5z M33 18h5v4h-5z" />
              <path d="M22 18h1v3h-1z M35 18h1v3h-1z" fill="#101b24" />
              <path d="M20 18h1v1h-1z M33 18h1v1h-1z" fill="#d9ffec" />
            </g>
          )}
          {alert && (
            <path
              className="pet-badge"
              d={
                critical
                  ? 'M20 15h2v1h2v1h-2v-1h-2z M34 16h2v-1h2v1h-2v1h-2z'
                  : 'M20 16h4v1h-4z M34 16h4v1h-4z'
              }
            />
          )}
          <path d="M28 21h2v1h-2z" fill="#6df7b0" />
          <path
            d={
              critical
                ? 'M27 23h4v3h-4z'
                : alert
                  ? 'M28 23h2v2h-2z'
                  : 'M25 23h1v1h2v-1h2v1h2v-1h1v2h-8z'
            }
            fill="#9cb7bd"
          />
          <path d="M11 21h6v1h-6z M12 24h5v1h-5z M41 21h6v1h-6z M41 24h5v1h-5z" fill="#5c8b8e" />
        </g>
      </g>
      <g className="pet-laptop">
        <path d="M18 30h22v10h3v2H15v-2h3z" fill="#101b24" />
        <path d="M19 31h20v9H19z" fill="#548d83" />
        <path d="M20 32h18v7H20z" fill="#0b2024" />
        <g opacity={asleep ? '.35' : '1'}>
          <path d="M22 33h1v1h1v1h-1v1h-1v-1h1v-1h-1z" fill="#6df7b0" />
          <path className="pet-terminal-cursor" d="M26 35h3v1h-3z" fill="#6df7b0" />
          <path d="M22 37h5v1h-5z M28 37h4v1h-4z" fill="#2e9b80" />
          {alert ? (
            <path className="pet-badge" d="M34 33h2v3h-2z M34 37h2v1h-2z" />
          ) : (
            <path d="M33 33h3v1h-3z M31 35h5v1h-5z" fill="#38665f" />
          )}
        </g>
        <path d="M16 40h26v1H16z" fill="#6c9997" />
        <path className="pet-badge" d="M27 40h5v1h-5z" />
        <path d="M15 33h3v4h-3z M40 33h3v4h-3z" fill="#45616a" />
        <path d="M15 33h3v1h-3z M40 33h3v1h-3z" fill="#8daaaa" />
      </g>
      {asleep ? (
        <path
          className="pet-emote"
          d="M48 6h5v1h-1v1h-1v1h-1v1h3v1h-5v-1h1V9h1V8h1V7h-3z"
          fill="#76bca7"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={48} y={6} />
      ) : (
        <g className="pet-emote" fill="#6df7b0">
          <path d="M6 13h2v1H6v1H5v1h1v1h2v1H6v-1H4v-3h2z" />
          <path d="M49 7h2v1h2v3h-2v1h-2v-1h2v-1h1V9h-1V8h-2z" />
        </g>
      )}
    </svg>
  );
}

function SpaceCat({ mood }: { mood: PetMood }) {
  const asleep = mood === 'resting';
  const critical = mood === 'critical';
  const alert = mood === 'warning' || mood === 'critical';
  return (
    <svg
      className="pet-sprite pet-space-sprite"
      viewBox="0 0 56 44"
      shapeRendering="crispEdges"
      role="img"
      aria-label="太空猫猫"
    >
      <path d="M17 42h23v1H17z M21 43h15v1H21z" fill="#717aab" opacity=".18" />
      <g className="pet-space-float">
        <g className="pet-tail">
          <path d="M38 32h7v-3h3v-5h4v2h2v5h-2v4h-5v3h-9z" fill="#535d85" />
          <path d="M39 33h7v-3h3v-5h2v3h2v2h-2v4h-5v3h-7z" fill="#eaf4ff" />
          <path d="M49 25h2v3h2v2h-4z M41 34h4v2h-4z" fill="#baa9e8" />
        </g>
        <g className="pet-jetpack">
          <path d="M12 27h8v10h-8z M37 27h8v10h-8z" fill="#535d85" />
          <path d="M13 28h6v7h-6z M38 28h6v7h-6z" fill="#a8b8dc" />
          <path d="M13 29h2v5h-2z M42 29h2v5h-2z" fill="#eaf4ff" />
          <path d="M14 35h4v2h-4z M39 35h4v2h-4z" fill="#697698" />
          {!asleep && (
            <g className="pet-thruster">
              <path d="M14 37h4v3h-1v2h-2v-2h-1z M39 37h4v3h-1v2h-2v-2h-1z" fill="#7bd9f5" />
              <path d="M15 37h2v3h-2z M40 37h2v3h-2z" fill="#e5fbff" />
            </g>
          )}
        </g>
        <g className="pet-space-suit">
          <path d="M20 26h17v3h3v9h-2v3h-8v-3h-3v3h-8v-3h-2v-9h3z" fill="#535d85" />
          <path d="M21 27h15v3h3v7h-2v3h-6v-4h-5v4h-6v-3h-2v-7h3z" fill="#edf4ff" />
          <path d="M19 30h3v7h-3z M35 30h3v7h-3z" fill="#c1c9e5" />
          <path d="M19 38h7v2h-7z M31 38h7v2h-7z" fill="#a79bd2" />
          <path d="M16 29h3v7h-3z M38 29h3v7h-3z" fill="#535d85" />
          <path d="M16 30h3v4h-3z M38 30h3v4h-3z" fill="#f7f8ff" />
          <path d="M16 33h3v1h-3z M38 33h3v1h-3z" fill="#baa9e8" />
          <path d="M24 30h10v6H24z" fill="#697698" />
          <path d="M25 31h8v4h-8z" fill="#263e61" />
          <path className="pet-badge" d="M26 32h3v2h-3z" />
          <path d="M30 32h2v1h-2z M30 34h2v1h-2z" fill="#e9c77f" />
        </g>
        <g className="pet-helmet">
          <path d="M20 2h17v1h5v3h3v4h2v12h-2v4h-4v3H16v-3h-4v-4h-2V10h2V6h3V3h5z" fill="#535d85" />
          <path d="M20 3h17v1h5v3h2v4h2v10h-2v4h-4v3H17v-3h-4v-4h-2V11h2V7h3V4h4z" fill="#d7f3ff" />
          <path d="M20 5h17v1h4v3h2v4h1v7h-2v4h-4v2H19v-2h-4v-4h-2v-7h1V9h3V6h3z" fill="#47789e" />
          <path
            d="M21 6h16v1h4v4h2v9h-2v3h-4v2H20v-2h-4v-3h-2v-7h2V9h2V7h3z"
            fill="#90d2e8"
            opacity=".4"
          />
          <path d="M17 9h3v2h3v3h10v-3h3V9h3v11h-2v3h-4v2H23v-2h-4v-3h-2z" fill="#48546e" />
          <path d="M18 10h1v2h3v3h12v-3h3v-2h1v10h-2v2h-3v2H23v-2h-3v-2h-2z" fill="#fff6e5" />
          <path d="M19 12h1v1h2v3h-3z M36 12h1v4h-3v-3h2z" fill="#d9a9c3" />
          <path d="M26 15h2v2h-2z M29 15h2v2h-2z" fill="#d9cbdc" />
          {asleep ? (
            <path d="M21 19h4v1h-4z M32 19h4v1h-4z" fill="#535d85" />
          ) : (
            <g className="pet-eyes">
              <path d="M22 17h3v4h-3z M32 17h3v4h-3z" fill="#384865" />
              <path d="M22 17h1v1h-1z M32 17h1v1h-1z" fill="#fff" />
            </g>
          )}
          {alert && (
            <path
              d={
                critical
                  ? 'M21 15h2v1h2v1h-2v-1h-2z M32 16h2v-1h2v1h-2v1h-2z'
                  : 'M21 16h4v1h-4z M32 16h4v1h-4z'
              }
              fill="#535d85"
            />
          )}
          <path d="M27 20h3v1h-3z" fill="#c18d9e" />
          <path
            d={
              critical
                ? 'M26 22h5v3h-5z'
                : alert
                  ? 'M27 22h3v2h-3z'
                  : 'M25 22h1v1h2v-1h1v1h2v-1h1v2h-7z'
            }
            fill="#535d85"
          />
          <path d="M19 21h3v1h-3z M35 21h3v1h-3z" fill="#e6b7be" />
          <path
            d="M17 7h3v1h-2v2h-2v4h-1v-4h1V8h1z M38 21h2v-3h1v4h-3z"
            fill="#fff"
            opacity=".75"
          />
          <path d="M17 26h23v3H17z" fill="#535d85" />
          <path d="M18 26h21v1H18z" fill="#f7f8ff" />
          <path className="pet-badge" d="M24 27h9v1h-9z" />
          <path d="M10 14h3v7h-3z M44 14h3v7h-3z" fill="#a8b8dc" />
          <path className="pet-badge" d="M11 16h1v3h-1z M45 16h1v3h-1z" />
        </g>
      </g>
      <path d="M5 6h1v2h2v1H6v2H5V9H3V8h2z M4 24h2v2H4z" fill="#e9c77f" />
      {asleep ? (
        <path
          className="pet-emote"
          d="M49 6h5v1h-1v1h-1v1h-1v1h3v1h-5v-1h1V9h1V8h1V7h-3z"
          fill="#baa9e8"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={49} y={5} />
      ) : (
        <g className="pet-emote">
          <path d="M49 6h4v1h1v4h-1v1h-4v-1h-1V7h1z" fill="#baa9e8" />
          <path d="M49 6h3v1h-2v2h-2V7h1z" fill="#e1d5ff" />
          <path d="M53 6h2v2h-2v2h-3v2h-3v-2h3V8h3z" fill="#e9c77f" />
        </g>
      )}
    </svg>
  );
}

export function PixelNoticeIcon({
  kind,
}: {
  kind: 'warning' | 'critical' | 'positive' | 'attention';
}) {
  const paths = {
    warning: 'M7 1h2v2h2v3h2v3h2v5H1V9h2V6h2V3h2z M7 5v4h2V5z M7 11v2h2v-2z',
    critical: 'M5 1h6v2h2v2h2v6h-2v2h-2v2H5v-2H3v-2H1V5h2V3h2z M7 4v5h2V4z M7 11v2h2v-2z',
    positive: 'M12 3h3v3h-2v2h-2v2H9v2H7v2H5v-2H3v-2H1V7h3v2h2v2h1V9h2V7h2V5h1z',
    attention: 'M2 2h12v10H7v2H5v1H3v-3H2z M4 6v2h2V6z M7 6v2h2V6z M10 6v2h2V6z',
  };
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 16 16"
      fill="currentColor"
      fillRule="evenodd"
      shapeRendering="crispEdges"
      aria-hidden="true"
    >
      <path d={paths[kind]} />
    </svg>
  );
}

export function PixelIcon({ kind }: { kind: 'open' | 'sound' | 'muted' | 'style' | 'close' }) {
  const paths = {
    open: 'M2 5h5v1H3v7h7V9h1v5H2z M8 2h6v6h-2V5l-5 5-1-1 5-5H8z',
    sound: 'M2 6h3l4-3v10l-4-3H2z M11 5h1v6h-1z M13 3h1v10h-1z',
    muted: 'M2 6h3l4-3v10l-4-3H2z M11 5h1v1h1V5h1v2h-1v1h1v2h-1V9h-1v1h-1V8h1V7h-1z',
    style:
      'M2 2h3v1h1v1h4V3h1V2h3v9h-1v2h-2v1H5v-1H3v-2H2z M3 4v6h1v2h2v1h4v-1h2v-2h1V4h-1v2h-2V5H6v1H4V4z M5 8h2v2H5z M9 8h2v2H9z M7 11h2v1H7z',
    close: 'M3 3h2v2h2v2h2V5h2V3h2v2h-2v2H9v2h2v2h2v2h-2v-2H9V9H7v2H5v2H3v-2h2V9h2V7H5V5H3z',
  };
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="currentColor"
      shapeRendering="crispEdges"
      aria-hidden="true"
    >
      <path d={paths[kind]} />
    </svg>
  );
}
