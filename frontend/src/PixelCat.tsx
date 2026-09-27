import type { PetStyle } from './monitoring-api';

export type PetMood = 'watching' | 'resting' | 'warning' | 'critical';

// The 84×66 grid renders at 2× in the desktop pet, with integer-aligned details.
export function PixelCat({ mood, style = 'classic' }: { mood: PetMood; style?: PetStyle }) {
  if (style === 'space') return <SpaceCat mood={mood} />;
  return style === 'hacker' ? <HackerCat mood={mood} /> : <ClassicCat mood={mood} />;
}

function AlertEmote({ mood, x, y }: { mood: PetMood; x: number; y: number }) {
  return (
    <g transform={`translate(${x} ${y})`}>
      <g className={`pet-emote pet-alert-emote-${mood}`}>
        <path d="M0 0h3v9H0zM0 12h3v3H0z" />
        {mood === 'critical' && <path d="M6 0h3v9H6zM6 12h3v3H6z" />}
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
      viewBox="0 0 84 66"
      shapeRendering="crispEdges"
      role="img"
      aria-label="像素监控小猫"
    >
      <path d="M23 62h42v1H23zM27 63h33v2H27z" fill="#302b3d" opacity=".18" />
      <g className="pet-tail">
        <path d="M57 47h11v-5h4v-10h6v3h3v12h-3v6h-6v4H57z" fill="#493b42" />
        <path d="M59 48h10v-4h5v-11h3v5h3v7h-3v6h-6v5h-12z" fill="#dc955b" />
        <path d="M74 33h3v5h3v3h-2v1h-4z" fill="#fff0cf" />
        <path d="M75 34h1v5h2v1h-3zM70 45h2v5h-2v2h-6v-1h5v-3h1z" fill="#f6c68a" />
        <path d="M73 43h6v2h-6zM68 49h3v3h-3zM62 52h2v3h-2z" fill="#b66c48" />
        <path d="M60 53h11v-3h6v3h-5v3h-12z" fill="#b66c48" />
        <path d="M60 55h10v-1h2v1h-1v1H60z" fill="#955d48" />
      </g>
      <g className="pet-cat-body">
        <path d="M29 41h27v4h4v11h3v6H21v-6h3v-11h5z" fill="#493b42" />
        <path d="M30 42h24v5h5v10h3v3H23v-3h3v-10h4z" fill="#e7a263" />
        <path d="M27 47h3v8h-2v2h-3v-1h1v-6h1z" fill="#f6c68a" />
        <path d="M55 47h3v9h3v4h-9v-2h3zM34 59h17v1H34z" fill="#c6804f" />
        <path d="M37 46h11v3h3v3h2v6h-2v1H35v-1h-2v-6h2v-3h2z" fill="#fff0cf" />
        <path d="M38 49h9v1h2v3h-3v-1h-7v2h-3v-3h2z" fill="#fff8e8" />
        <path d="M34 56h2v1h14v-1h2v2h-1v1H35v-1h-1z" fill="#e8c99b" />
        <path d="M26 50h5v2h-3v1h-2zM26 54h3v1h-3zM55 50h3v2h-3z" fill="#b66c48" />
        <path d="M24 57h3v-1h5v1h2v3H23v-2h1zM52 56h5v1h3v3H51v-3h1z" fill="#fff0cf" />
        <path d="M24 57h7v1h-7zM52 57h5v1h-5z" fill="#fff8e8" />
        <path d="M27 59h1v1h-1zM30 59h1v1h-1zM54 59h1v1h-1zM57 59h1v1h-1z" fill="#bb997c" />
        <path d="M26 42h33v4h-3v1h-5v7h-3v-7H30v-1h-4z" fill="#38675e" />
        <path d="M27 42h30v2H27zM52 46h4v7h-2v-2h-2z" fill="#8fc4a3" />
        <path d="M29 43h22v1H29zM53 46h2v3h-2z" fill="#c2e1b5" />
        <path d="M52 52h2v2h-2zM55 52h1v2h-1z" fill="#38675e" />
        <path d="M40 45h6v1h1v4h-1v1h-6v-1h-1v-4h1z" fill="#986b43" />
        <path d="M40 46h6v4h-6z" fill="#f3d486" />
        <path className="pet-badge" d="M42 47h3v2h-3z" />
        <path d="M40 46h2v1h-2z" fill="#fff4bd" />
        <g className="pet-head">
          <path
            d="M18 8h7v2h3v3h3v3h7v-1h5v1h10v-2h4v-3h4V8h8v20h1v5h2v2h-4v4h-3v2h-5v3H26v-2h-5v-3h-3v-4h-3v-2h2V21h1z"
            fill="#493b42"
          />
          <path
            d="M20 10h4v2h3v3h3v3h9v-1h3v1h12v-2h5v-3h4v-3h4v19h1v5h2v1h-4v3h-3v2h-5v2H28v-2h-6v-2h-2v-4h-3v-1h2V22h1z"
            fill="#edb477"
          />
          <path d="M20 10h4v2h3v3h3v3h9v-1h3v1h12v2H30v-1h-3v-3h-4v-3h-2v11h-1z" fill="#ffd59b" />
          <path d="M65 12h2v17h1v5h2v1h-4v3h-3v2h-5v2H28v-2h29v-2h5v-3h2V22h1z" fill="#cf8d59" />
          <path d="M22 13h2v3h3v3h2v4h-7zM64 13h2v10h-8v-4h3v-3h3z" fill="#ad6d5e" />
          <path d="M23 16h1v2h3v3h1v1h-5zM64 16h1v6h-6v-2h3v-2h2z" fill="#efa993" />
          <path d="M23 19h1v2h2v1h-3zM64 19h1v3h-3v-1h2z" fill="#ffd1ad" />
          <path
            d="M33 20h4v3h-1v2h-2v-1h-1zM41 18h5v5h-1v3h-2v-2h-2zM51 20h3v4h-1v1h-2z"
            fill="#c6804f"
          />
          <path d="M34 20h1v3h-1zM42 19h1v4h-1zM51 20h1v3h-1z" fill="#d9955e" />
          <path
            d="M20 27h5v2h-3v1h-2zM20 32h4v2h-2v1h-2zM62 27h5v2h-5zM63 32h4v2h-4z"
            fill="#c6804f"
          />
          <path d="M37 30h13v2h5v2h7v3h-3v3h-5v1H31v-1h-6v-3h-2v-3h9v-2h5z" fill="#fff0cf" />
          <path d="M39 31h9v1h4v2h3v2H34v-2h5z" fill="#fff8e8" />
          <g fill="#493b42">
            {asleep ? (
              <path d="M29 29h2v2h4v-1h2v2h-2v1h-5v-1h-1zM51 30h2v1h4v-2h2v3h-1v1h-5v-1h-2z" />
            ) : (
              <g className="pet-eyes">
                <path d="M30 26h5v1h1v6h-1v1h-5v-1h-1v-6h1zM53 26h5v1h1v6h-1v1h-5v-1h-1v-6h1z" />
                <path d="M33 30h2v3h-3v-1h1zM56 30h2v3h-3v-1h1z" fill="#81a58e" />
                <path d="M30 27h2v2h-2zM53 27h2v2h-2z" fill="#fff8e8" />
                <path d="M34 31h1v1h-1zM57 31h1v1h-1z" fill="#c2d9ae" />
              </g>
            )}
            {alert && (
              <path
                d={
                  critical
                    ? 'M29 24h2v-1h3v-1h2v2h-4v1h-3zM52 22h2v1h3v1h2v1h-3v-1h-4z'
                    : 'M29 24h6v1h-6zM53 24h6v1h-6z'
                }
              />
            )}
            <path
              d={
                critical
                  ? 'M41 37h6v1h1v3h-8v-3h1z'
                  : alert
                    ? 'M42 37h4v3h-4z'
                    : 'M38 37h1v1h4v-2h2v2h4v-1h1v2h-5v-1h-2v1h-5z'
              }
            />
          </g>
          {critical && <path d="M42 40h4v1h-4z" fill="#e8a090" />}
          <path d="M41 34h6v1h-1v1h-4v-1h-1z" fill="#ba736b" />
          <path d="M42 34h2v1h-2z" fill="#edb0a2" />
          <path d="M26 35h5v1h-5zM57 35h5v1h-5z" fill="#e8a090" />
          <path d="M33 35h1v1h-1zM35 37h1v1h-1zM53 35h1v1h-1zM51 37h1v1h-1z" fill="#d1a378" />
          <path
            d="M14 31h4v1h5v1h-6v-1h-3zM15 37h5v-1h3v1h-3v1h-5zM65 32h5v-1h4v1h-3v1h-6zM65 36h3v1h5v1h-5v-1h-3z"
            fill="#806052"
          />
        </g>
      </g>
      {asleep ? (
        <path
          className="pet-emote"
          d="M71 12h7v2h-1v1h-2v2h-1v1h4v2h-7v-2h1v-1h2v-2h1V14h-4z"
          fill="#a7b4c9"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={72} y={11} />
      ) : (
        <path className="pet-emote" d="M74 12h1v3h3v2h-3v3h-1v-3h-3v-2h3z" fill="#8fc4a3" />
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
      viewBox="0 0 84 66"
      shapeRendering="crispEdges"
      role="img"
      aria-label="黑客猫猫"
    >
      <path d="M24 62h39v1H24zM28 63h31v1H28z" fill="#252b3c" opacity=".2" />
      <g className="pet-tail">
        <path d="M58 49h8v-2h5v-4h2v-8h2v-2h4v2h2v11h-2v5h-4v4h-6v3H58z" fill="#2b2c3b" />
        <path d="M59 50h8v-2h5v-4h2v-8h2v-2h2v2h2v9h-2v5h-4v4h-6v2h-9z" fill="#53566b" />
        <path d="M75 36h3v3h2v4h-3v-1h-2z" fill="#b7bbc7" />
        <path d="M75 36h2v3h1v1h-2v-1h-1zM72 45h2v4h-3v2h-4v1h-5v-1h4v-2h5v-2h1z" fill="#73778a" />
        <path d="M77 46h1v3h-4v4h-6v2h-9v1h9v-2h6v-4h3z" fill="#414354" />
      </g>
      <g className="pet-cat-body">
        <path d="M30 40h25v3h5v6h2v8h-2v4H25v-4h-2v-8h2v-6h5z" fill="#2b2c3b" />
        <path d="M31 42h23v3h5v11h-2v3H28v-3h-3V46h6z" fill="#24333c" />
        <path d="M27 46h3v9h-3zM55 46h3v9h-3z" fill="#354b53" />
        <path d="M31 42h5v2h4v2h6v-2h4v-2h5v3h-5v2h-4v1h-6v-1h-4v-2h-5z" fill="#13232b" />
        <path
          d="M30 44h2v1h2v2h-1v-1h-3zM52 45h2v-1h2v2h-3v1h-1zM34 45h1v5h-1zM51 45h1v5h-1z"
          fill="#74cfa4"
        />
        <path d="M34 50h1v2h-1zM51 50h1v2h-1z" fill="#c6efd7" />
        <path d="M26 58h8v2h-8zM52 58h8v2h-8z" fill="#b7bbc7" />
        <path d="M28 59h1v1h-1zM31 59h1v1h-1zM54 59h1v1h-1zM57 59h1v1h-1z" fill="#73778a" />
        <g className="pet-head">
          <g className="pet-hood">
            <path
              d="M34 3h18v2h7v4h5v5h4v6h3v13h-2v6h-4v4h-6v3H27v-2h-7v-4h-4v-6h-2V21h3v-7h5V9h5V6h7z"
              fill="#13232b"
            />
            <path
              d="M35 4h16v2h7v4h5v5h4v6h2v12h-2v6h-4v3h-6v2H28v-2h-7v-3h-3v-6h-2V22h3v-7h4v-5h5V7h7z"
              fill="#24333c"
            />
            <path d="M35 5h15v1H35v2h-6v3h-5v5h-4v7h-2v8h-1v-9h2v-7h4v-5h5V7h7z" fill="#526671" />
            <path d="M51 7h6v4h5v5h4v7h2v9h-1v-8h-3v-7h-4v-5h-4V9h-5z" fill="#354b53" />
            <path
              d="M34 10h16v1H34v2h-5v-1h5zM19 25h1v7h1v4h3v3h-2v-2h-3zM64 36h2v3h-3v2h-4v-1h4v-2h1z"
              fill="#4f967f"
            />
            <path d="M40 6h6v1h-6zM35 8h3v1h-3z" fill="#74cfa4" />
          </g>
          <path
            d="M20 7h5v2h3v3h3v4h7v-1h3v-2h4v2h3v1h7v-4h3V9h3V7h5v14h2v5h2v7h-2v4h-3v3h-5v2h-7v2H33v-2h-7v-2h-4v-3h-3v-4h-2v-7h2v-5h1z"
            fill="#2b2c3b"
          />
          <path
            d="M21 9h3v2h3v3h3v4h9v-1h3v-2h2v2h3v1h9v-4h3v-3h3V9h3v13h2v5h1v6h-2v4h-3v2h-5v2h-7v1H35v-1h-7v-2h-5v-3h-2v-4h-2v-5h2z"
            fill="#53566b"
          />
          <path d="M21 9h3v2h3v3h3v4h9v-1h3v-2h2v3h-3v2H29v-4h-3v-3h-3v9h-2z" fill="#73778a" />
          <path
            d="M62 11h3v11h2v5h1v6h-2v4h-3v2h-5v2h-7v1H35v-1h16v-2h6v-2h5v-4h2V23h-2z"
            fill="#414354"
          />
          <path d="M23 13h2v3h3v3h1v5h-6zM62 13h1v11h-6v-5h2v-3h3z" fill="#98778b" />
          <path d="M24 16h1v2h2v3h1v2h-4zM61 17h1v6h-4v-2h2v-3h1z" fill="#d6a5b2" />
          <path d="M24 19h1v3h-1zM61 20h1v2h-1z" fill="#edc4c7" />
          <path d="M36 20h3v2h-1v1h-2zM42 19h3v4h-1v1h-1v-2h-1zM49 20h2v3h-2z" fill="#65697f" />
          <path d="M30 17h9v-1h3v-1h2v2h3v1h9v2H30z" fill="#24333c" />
          <path d="M31 18h8v-1h2v1h5v1h9v1H31z" fill="#354b53" />
          <path d="M23 28h3v1h-3zM22 31h4v1h-4zM61 28h3v1h-3zM61 31h4v1h-4z" fill="#73778a" />
          <path d="M37 32h12v2h5v2h4v2h-3v2h-6v1H36v-1h-5v-2h-3v-2h5v-2h4z" fill="#b7bbc7" />
          <path d="M38 33h10v2h4v3H34v-2h4z" fill="#d3d4d8" />
          {asleep ? (
            <path
              d="M28 29h2v2h5v-1h2v2h-2v1h-5v-1h-2zM49 30h2v1h5v-2h2v3h-2v1h-5v-1h-2z"
              fill="#252838"
            />
          ) : (
            <g className="pet-eyes">
              <path
                d="M29 25h6v1h2v7h-2v1h-6v-1h-1v-7h1zM51 25h6v1h1v7h-1v1h-6v-1h-2v-7h2z"
                fill="#252838"
              />
              <path d="M33 29h3v3h-1v1h-4v-2h2zM54 29h3v3h-1v1h-4v-2h2z" fill="#74cfa4" />
              <path d="M29 26h3v3h-3zM50 26h3v3h-3z" fill="#f0eee6" />
              <path d="M34 30h1v2h-1zM55 30h1v2h-1z" fill="#c8e1bd" />
            </g>
          )}
          <g className="pet-terminal-glasses">
            <path
              d="M27 24h11v1h1v9h-1v1H27v-1h-1v-9h1zM27 25v9h11v-9zM49 24h10v1h1v9h-1v1H49v-1h-1v-9h1zM49 25v9h10v-9z"
              fill="#132b2f"
              fillRule="evenodd"
            />
            <path d="M38 27h11v2H38zM23 25h3v2h-3zM60 25h3v2h-3z" fill="#132b2f" />
            <path
              d="M27 24h11v1H27zM26 26h1v6h-1zM49 24h10v1H49zM59 26h1v6h-1zM39 27h9v1h-9z"
              fill="#74cfa4"
            />
            <path d="M28 34h9v1h-9zM50 34h8v1h-8z" fill="#4f967f" />
            <path d="M36 26h1v2h-1zM57 26h1v2h-1z" fill="#a6d4c5" />
          </g>
          {alert && (
            <path
              d={
                critical
                  ? 'M28 23h3v-1h3v-1h3v2h-4v1h-5zM49 21h3v1h3v1h3v1h-5v-1h-4z'
                  : 'M28 23h8v1h-8zM50 23h8v1h-8z'
              }
              fill="#b1bdc7"
            />
          )}
          <path d="M40 34h6v1h-1v1h-4v-1h-1z" fill="#ba8e9f" />
          <path d="M41 34h2v1h-2z" fill="#e2b5bf" />
          <path
            d={
              critical
                ? 'M41 37h4v1h1v3h-6v-3h1z'
                : alert
                  ? 'M42 37h3v3h-3z'
                  : 'M37 37h1v1h4v-2h2v2h4v-1h1v2h-5v-1h-2v1h-5z'
            }
            fill="#414354"
          />
          {critical && <path d="M42 40h2v1h-2z" fill="#d6a5b2" />}
          <path d="M26 34h5v1h-5zM56 34h5v1h-5z" fill="#98778b" />
          <path d="M33 36h1v1h-1zM52 36h1v1h-1z" fill="#8a8e9f" />
          <path
            d="M15 32h5v1h6v1h-6v-1h-5zM17 37h5v-1h4v1h-4v1h-5zM61 33h6v-1h5v1h-5v1h-6zM61 36h4v1h5v1h-5v-1h-4z"
            fill="#8a8e9f"
          />
        </g>
      </g>
      <g className="pet-laptop">
        <path d="M28 46h30v1h1v12h3v1h1v3H24v-3h1v-1h2V47h1z" fill="#13232b" />
        <path d="M29 47h28v12H29z" fill="#3c625d" />
        <path d="M29 47h28v1H29zM29 48h1v10h-1z" fill="#74cfa4" />
        <path d="M30 48h26v10H30z" fill="#0e2528" />
        <path d="M31 49h24v1H31z" fill="#203c3c" />
        <g opacity={asleep ? '.35' : '1'}>
          <path d="M32 50h1v1h1v1h1v1h-1v1h-1v-1h1v-1h-1v-1h-1z" fill="#6df7b0" />
          <path className="pet-terminal-cursor" d="M36 53h4v1h-4z" fill="#b8f6d0" />
          <path d="M32 56h5v1h-5zM39 56h8v1h-8zM42 51h4v1h-4z" fill="#53b68a" />
          {alert ? (
            <path className="pet-badge" d="M51 50h2v4h-2zM51 56h2v1h-2z" />
          ) : (
            <path d="M48 51h5v1h-5zM43 53h10v1H43zM49 56h4v1h-4z" fill="#348268" />
          )}
        </g>
        <path d="M26 60h35v1h1v1H25v-1h1z" fill="#354b53" />
        <path d="M28 60h9v1h-9zM50 60h8v1h-8z" fill="#67978a" />
        <path d="M39 60h9v1h-9z" fill="#13232b" />
        <path className="pet-badge" d="M58 61h2v1h-2z" />
        <path d="M25 49h4v1h1v4h-1v1h-5v-5h1zM57 49h4v1h1v5h-5v-1h-1v-4h1z" fill="#2b2c3b" />
        <path d="M25 50h4v4h-4zM57 50h4v4h-4z" fill="#b7bbc7" />
        <path d="M25 50h3v1h-3zM57 50h3v1h-3z" fill="#d3d4d8" />
        <path d="M26 53h1v1h-1zM58 53h1v1h-1z" fill="#73778a" />
      </g>
      {asleep ? (
        <path
          className="pet-emote"
          d="M72 9h8v2h-2v1h-1v2h-2v1h5v2h-8v-2h2V14h1V12h2V11h-5z"
          fill="#91baa7"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={72} y={9} />
      ) : (
        <g className="pet-emote" fill="#74cfa4">
          <path d="M73 10h1v1h1v1h1v1h-1v1h-1v1h-1v-1h1v-1h1v-1h-1v-1h-1zM78 15h4v1h-4z" />
          <path
            d="M4 22h3v4H4zM5 23v2h1v-2zM10 22h1v4h-1zM5 29h1v4H5zM9 29h3v4H9zM10 30v2h1v-2z"
            fillRule="evenodd"
            opacity=".65"
          />
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
      viewBox="0 0 84 66"
      shapeRendering="crispEdges"
      role="img"
      aria-label="太空猫猫"
    >
      <path d="M26 63h34v2H26zM32 65h22v1H32z" fill="#717aab" opacity=".18" />
      <g className="pet-space-float">
        <g className="pet-tail">
          <path d="M57 48h11v-4h4v-8h6v3h3v8h-3v6h-7v4h-14z" fill="#535d85" />
          <path d="M59 50h10v-5h5v-7h3v4h3v3h-3v6h-8v5h-10z" fill="#eaf4ff" />
          <path d="M74 38h3v4h3v3h-6zM62 51h6v3h-6z" fill="#baa9e8" />
          <path d="M74 38h2v4h2v1h-3v-2h-1zM70 46h2v4h-2v2h-1v-3h1z" fill="#fff" />
          <path d="M75 46h2v3h-2zM68 52h2v2h-2zM60 55h8v1h-8z" fill="#a8b8dc" />
        </g>
        <g className="pet-jetpack">
          <path d="M18 41h12v15h-12zM56 41h12v15h-12z" fill="#535d85" />
          <path d="M20 42h9v11h-9zM57 42h9v11h-9z" fill="#a8b8dc" />
          <path d="M20 44h3v7h-3zM63 44h3v7h-3z" fill="#eaf4ff" />
          <path d="M20 43h8v1h-8zM58 43h7v1h-7z" fill="#fff" />
          <path d="M20 46h3v1h-3zM20 49h3v1h-3zM63 46h3v1h-3zM63 49h3v1h-3z" fill="#8195ba" />
          <path d="M21 53h6v3h-6zM59 53h6v3h-6z" fill="#697698" />
          <path d="M22 54h4v1h-4zM60 54h4v1h-4z" fill="#344e74" />
          {!asleep && (
            <g className="pet-thruster">
              <path
                d="M21 56h6v3h-1v2h-1v2h-2v-2h-1v-2h-1zM59 56h6v3h-1v2h-1v2h-2v-2h-1v-2h-1z"
                fill="#7bd9f5"
              />
              <path d="M22 56h4v2h-1v2h-2v-2h-1zM60 56h4v2h-1v2h-2v-2h-1z" fill="#e5fbff" />
              <path d="M23 56h2v1h-2zM61 56h2v1h-2z" fill="#fff" />
            </g>
          )}
        </g>
        <g className="pet-space-suit">
          <path d="M30 39h26v5h4v13h-3v5h-12v-5h-4v5h-12v-5h-3v-13h4z" fill="#535d85" />
          <path d="M32 41h22v4h5v11h-3v4h-9v-6h-8v6h-9v-4h-3v-11h5z" fill="#edf4ff" />
          <path d="M29 45h4v11h-4zM53 45h4v11h-4z" fill="#c1c9e5" />
          <path d="M33 44h3v10h-1v2h-3v-1h1zM51 44h2v10h2v2h-3v-2h-1z" fill="#a8b8dc" />
          <path d="M34 44h1v8h-1zM52 44h1v8h-1z" fill="#fff" />
          <path d="M39 54h8v1h-8zM32 56h7v1h-7zM47 56h8v1h-8z" fill="#8195ba" />
          <path d="M29 57h10v3h-10zM47 57h10v3h-10z" fill="#a79bd2" />
          <path d="M30 57h7v1h-7zM48 57h7v1h-7z" fill="#d9d1f3" />
          <path
            d="M29 60h10v1H29zM47 60h10v1H47zM32 59h1v1h-1zM35 59h1v1h-1zM50 59h1v1h-1zM53 59h1v1h-1z"
            fill="#697698"
          />
          <path d="M24 44h5v10h-5zM57 44h5v10h-5z" fill="#535d85" />
          <path d="M24 45h5v6h-5zM57 45h5v6h-5z" fill="#f7f8ff" />
          <path d="M24 50h5v1h-5zM57 50h5v1h-5z" fill="#baa9e8" />
          <path d="M25 46h1v3h-1zM58 46h1v3h-1z" fill="#fff" />
          <path d="M25 52h3v1h-3zM58 52h3v1h-3z" fill="#a8b8dc" />
          <path d="M36 45h15v9H36z" fill="#697698" />
          <path d="M37 46h13v1H37z" fill="#d7f3ff" />
          <path d="M37 47h13v6H37z" fill="#263e61" />
          <path d="M38 48h7v4h-7z" fill="#385b7f" />
          <path className="pet-badge" d="M38 50h2v-2h1v3h2v-2h2v1h-1v2h-4v-1h-2z" />
          <path d="M47 48h2v1h-2zM47 51h2v1h-2z" fill="#e9c77f" />
          <path d="M37 55h2v1h-2zM48 55h2v1h-2z" fill="#baa9e8" />
        </g>
        <g className="pet-helmet">
          <path d="M30 3h26v2h7v4h5v6h3v18h-3v6h-6v5H24v-5h-6v-6h-3V15h3V9h5V5h7z" fill="#535d85" />
          <path
            d="M30 5h26v1h7v5h3v6h3v15h-3v6h-6v4H26v-4h-6v-6h-3V17h3V11h4V6h6z"
            fill="#d7f3ff"
          />
          <path d="M30 5h23v1H30zM24 6h6v1h-5v4h-4v6h-2v7h-1v-7h2v-6h4z" fill="#fff" />
          <path d="M65 12h1v5h3v15h-3v6h-6v3h-4v-2h3v-3h5v-5h3V18h-2z" fill="#9cbad7" />
          <path
            d="M30 8h26v1h6v5h3v6h1v10h-3v6h-6v3H29v-3h-6v-6h-3v-10h1V14h5V9h4z"
            fill="#47789e"
          />
          <path
            d="M32 9h24v2h6v6h3v13h-3v5h-6v3H30v-3h-6v-5h-3v-10h3V14h3V11h5z"
            fill="#90d2e8"
            opacity=".4"
          />
          <path d="M24 30h3v4h5v3h24v-2h5v-4h2v4h-2v2h-4v1H30v-3h-6z" fill="#3e648b" />
          <path d="M26 14h5v3h4v4h15v-3h4v-4h5v15h1v3h-3v3h-6v3H35v-2h-6v-3h-3z" fill="#48546e" />
          <path d="M27 15h3v3h4v5h17v-4h4v-4h3v15h1v1h-3v3h-6v3H36v-2h-6v-3h-3z" fill="#fff6e5" />
          <path d="M28 16h2v2h4v5h-3v-2h-3zM55 16h2v7h-5v-3h3z" fill="#d9a9c3" />
          <path d="M28 18h1v2h2v2h-2v-1h-1zM55 18h1v3h-2v-1h1z" fill="#f9d5d4" />
          <path d="M56 24h2v6h1v1h-3v3h-6v3H36v-2h14v-2h4v-2h2z" fill="#d9cbdc" />
          <path d="M39 23h2v2h-2zM43 22h3v3h-1v1h-1v-1h-1zM48 23h2v2h-2z" fill="#d9cbdc" />
          <path d="M38 30h12v3H38zM35 33h18v2H35z" fill="#fffdf5" />
          {asleep ? (
            <path d="M32 28h1v2h4v-1h1v2h-5v-1h-1zM48 29h1v1h4v-2h1v2h-1v1h-5z" fill="#535d85" />
          ) : (
            <g className="pet-eyes">
              <path
                d="M33 26h4v1h1v4h-1v1h-4v-1h-1v-4h1zM49 26h4v1h1v4h-1v1h-4v-1h-1v-4h1z"
                fill="#384865"
              />
              <path d="M36 29h1v2h-2v-1h1zM52 29h1v2h-2v-1h1z" fill="#8eabd9" />
              <path d="M33 27h2v2h-2zM49 27h2v2h-2z" fill="#fff" />
            </g>
          )}
          {alert && (
            <path
              d={
                critical
                  ? 'M32 24h3v-1h3v1h-2v1h-4zM48 23h3v1h3v1h-4v-1h-2z'
                  : 'M32 24h6v1h-6zM48 24h6v1h-6z'
              }
              fill="#535d85"
            />
          )}
          <path d="M41 31h4v1h-1v1h-2v-1h-1z" fill="#c18d9e" />
          <path d="M41 31h2v1h-2z" fill="#edbfc9" />
          <path
            d={
              critical
                ? 'M41 34h4v1h1v2h-6v-2h1z'
                : alert
                  ? 'M42 34h3v2h-3z'
                  : 'M38 34h1v1h3v-2h2v2h3v-1h1v2h-4v-1h-2v1h-4z'
            }
            fill="#535d85"
          />
          {critical && <path d="M42 36h2v1h-2z" fill="#e6b7be" />}
          <path d="M29 32h4v1h-4zM53 32h4v1h-4z" fill="#e6b7be" />
          <path d="M34 33h1v1h-1zM51 33h1v1h-1z" fill="#b9a8be" />
          <path
            d="M30 10h9v1h-9zM26 11h4v1h-3v3h-3v6h-1v-6h1V12h2zM23 23h1v3h-1zM59 31h2v-5h1v6h-3z"
            fill="#fff"
            opacity=".75"
          />
          <path
            d="M51 11h5v2h3v3h-2v-2h-3v-1h-3zM60 18h2v4h-1v-3h-1z"
            fill="#b9e9f3"
            opacity=".5"
          />
          <path d="M26 39h34v5H26z" fill="#535d85" />
          <path d="M27 39h32v2H27z" fill="#f7f8ff" />
          <path d="M28 40h29v1H28z" fill="#a8b8dc" />
          <path className="pet-badge" d="M36 41h14v1h-14z" />
          <path d="M15 21h5v11h-5zM66 21h5v11h-5z" fill="#a8b8dc" />
          <path d="M15 22h1v8h-1zM66 22h1v8h-1z" fill="#edf4ff" />
          <path d="M18 22h2v10h-2zM69 22h2v10h-2zM29 42h3v1h-3zM54 42h3v1h-3z" fill="#697698" />
          <path className="pet-badge" d="M17 24h1v5h-1zM68 24h1v5h-1z" />
        </g>
      </g>
      <path d="M8 9h1v3h3v2H9v3H8V14H5V12h3zM6 36h3v3H6z" fill="#e9c77f" />
      {asleep ? (
        <path
          className="pet-emote"
          d="M74 9h7v2h-1v1h-2v2h-1v1h4v2h-7v-2h1V14h2V12h1V11h-4z"
          fill="#baa9e8"
        />
      ) : alert ? (
        <AlertEmote mood={mood} x={74} y={8} />
      ) : (
        <g className="pet-emote">
          <path d="M74 9h6v2h1v6h-1v1h-6v-1h-2V11h2z" fill="#baa9e8" />
          <path d="M74 9h4v2h-3v3h-3V11h2z" fill="#e1d5ff" />
          <path d="M80 9h3v3h-3v3h-5v3h-4v-3h4V12h5z" fill="#e9c77f" />
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
