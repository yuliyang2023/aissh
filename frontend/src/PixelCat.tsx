export type PetMood = 'watching' | 'resting' | 'warning' | 'critical';

// Integer coordinates and a 3× display scale keep every sprite edge crisp.
export function PixelCat({ mood }: { mood: PetMood }) {
  const asleep = mood === 'resting';
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
            {alert && <path d="M19 16h4v1h-4z M35 16h4v1h-4z" />}
            <path d={alert ? 'M28 24h3v2h-3z' : 'M25 24h1v1h3v-1h1v1h3v-1h1v2h-9z'} />
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
        <path className="pet-emote pet-badge" d="M49 7h2v6h-2z M49 15h2v2h-2z" />
      ) : (
        <path className="pet-emote" d="M49 8h1v2h2v1h-2v2h-1v-2h-2v-1h2z" fill="#8fc4a3" />
      )}
    </svg>
  );
}

export function PixelIcon({ kind }: { kind: 'open' | 'sound' | 'muted' | 'close' }) {
  const paths = {
    open: 'M2 5h5v1H3v7h7V9h1v5H2z M8 2h6v6h-2V5l-5 5-1-1 5-5H8z',
    sound: 'M2 6h3l4-3v10l-4-3H2z M11 5h1v6h-1z M13 3h1v10h-1z',
    muted: 'M2 6h3l4-3v10l-4-3H2z M11 5h1v1h1V5h1v2h-1v1h1v2h-1V9h-1v1h-1V8h1V7h-1z',
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
