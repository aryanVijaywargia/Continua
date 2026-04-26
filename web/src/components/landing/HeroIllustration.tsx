import { useMediaQuery } from '../../hooks/useMediaQuery';

export function HeroIllustration() {
  const prefersReducedMotion = useMediaQuery(
    '(prefers-reduced-motion: reduce)'
  );

  return prefersReducedMotion ? (
    <StaticHeroIllustration />
  ) : (
    <AnimatedHeroIllustration />
  );
}

function AnimatedHeroIllustration() {
  return (
    <div className="hero-illustration relative mx-auto mt-20 w-full max-w-5xl px-6">
      <svg
        viewBox="0 0 900 350"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="w-full"
        role="img"
        aria-label="Animated visualization of durable agent execution flow"
      >
        <title>Durable execution network</title>
        <desc>
          Abstract flowing network showing checkpoint nodes, execution paths,
          and data particles traveling between processing stages
        </desc>

        <defs>
          <linearGradient id="hg-primary" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.12" />
            <stop offset="20%" stopColor="#0075d6" stopOpacity="0.85" />
            <stop offset="80%" stopColor="#0075d6" stopOpacity="0.85" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.12" />
          </linearGradient>

          <linearGradient id="hg-shift" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%">
              <animate
                attributeName="stop-color"
                values="#0075d6;#418400;#0075d6"
                dur="8s"
                repeatCount="indefinite"
              />
            </stop>
            <stop offset="100%">
              <animate
                attributeName="stop-color"
                values="#418400;#0075d6;#418400"
                dur="8s"
                repeatCount="indefinite"
              />
            </stop>
          </linearGradient>

          <radialGradient id="hg-glow-blue" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.5" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="hg-glow-green" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.4" />
            <stop offset="100%" stopColor="#418400" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="hg-glow-gold" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#fcd400" stopOpacity="0.4" />
            <stop offset="100%" stopColor="#fcd400" stopOpacity="0" />
          </radialGradient>

          <filter id="hg-blur">
            <feGaussianBlur in="SourceGraphic" stdDeviation="6" />
          </filter>
        </defs>

        <HeroBackdrop />
        <HeroPaths shiftStroke="url(#hg-shift)" />

        <g filter="url(#hg-blur)">
          <circle className="hero-glow" cx="50" cy="200" r="28" fill="url(#hg-glow-blue)" />
          <circle className="hero-glow hero-glow-d1" cx="340" cy="65" r="32" fill="url(#hg-glow-green)" />
          <circle className="hero-glow hero-glow-d2" cx="660" cy="335" r="28" fill="url(#hg-glow-gold)" />
          <circle className="hero-glow hero-glow-d3" cx="870" cy="200" r="32" fill="url(#hg-glow-blue)" />
        </g>

        <circle cx="50" cy="200" r="6" fill="#0075d6">
          <animate attributeName="r" values="6;7.5;6" dur="3s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;0.7;1" dur="3s" repeatCount="indefinite" />
        </circle>
        <circle cx="340" cy="65" r="7" fill="#418400">
          <animate attributeName="r" values="7;8.5;7" dur="3s" begin="0.6s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;0.7;1" dur="3s" begin="0.6s" repeatCount="indefinite" />
        </circle>
        <circle cx="500" cy="200" r="4.5" fill="#0075d6" opacity="0.7">
          <animate attributeName="r" values="4.5;5.8;4.5" dur="3s" begin="1.2s" repeatCount="indefinite" />
        </circle>
        <circle cx="660" cy="335" r="7" fill="#fcd400">
          <animate attributeName="r" values="7;8.5;7" dur="3s" begin="1.8s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="1;0.7;1" dur="3s" begin="1.8s" repeatCount="indefinite" />
        </circle>
        <circle cx="870" cy="200" r="8" fill="#1b1b1b">
          <animate attributeName="r" values="8;9.5;8" dur="3s" begin="2.4s" repeatCount="indefinite" />
        </circle>

        <HeroSecondaryNodes />

        <circle r="3" fill="#0075d6" opacity="0">
          <animateMotion
            dur="5s"
            repeatCount="indefinite"
            path="M 50,200 C 170,200 200,65 340,65 S 520,335 660,335 S 840,65 870,200"
          />
          <animate
            attributeName="opacity"
            values="0;0.9;0.9;0"
            keyTimes="0;0.05;0.92;1"
            dur="5s"
            repeatCount="indefinite"
          />
        </circle>
        <circle r="2.5" fill="#418400" opacity="0">
          <animateMotion
            dur="5s"
            repeatCount="indefinite"
            begin="1.7s"
            path="M 50,200 C 170,200 200,65 340,65 S 520,335 660,335 S 840,65 870,200"
          />
          <animate
            attributeName="opacity"
            values="0;0.75;0.75;0"
            keyTimes="0;0.05;0.92;1"
            dur="5s"
            repeatCount="indefinite"
            begin="1.7s"
          />
        </circle>
        <circle r="2" fill="#fcd400" opacity="0">
          <animateMotion
            dur="5s"
            repeatCount="indefinite"
            begin="3.3s"
            path="M 50,200 C 170,200 200,65 340,65 S 520,335 660,335 S 840,65 870,200"
          />
          <animate
            attributeName="opacity"
            values="0;0.65;0.65;0"
            keyTimes="0;0.05;0.92;1"
            dur="5s"
            repeatCount="indefinite"
            begin="3.3s"
          />
        </circle>
        <circle r="2" fill="#0075d6" opacity="0">
          <animateMotion
            dur="3s"
            repeatCount="indefinite"
            begin="0.5s"
            path="M 260,120 C 300,45 420,65 480,130"
          />
          <animate
            attributeName="opacity"
            values="0;0.6;0.6;0"
            keyTimes="0;0.1;0.85;1"
            dur="3s"
            repeatCount="indefinite"
            begin="0.5s"
          />
        </circle>

        <HeroAccents animated />
      </svg>
    </div>
  );
}

function StaticHeroIllustration() {
  return (
    <div className="hero-illustration relative mx-auto mt-20 w-full max-w-5xl px-6">
      <svg
        viewBox="0 0 900 350"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="w-full"
        role="img"
        aria-label="Static visualization of durable agent execution flow"
      >
        <title>Durable execution network</title>
        <desc>
          Abstract flowing network showing checkpoint nodes and execution paths
          without motion.
        </desc>

        <defs>
          <linearGradient id="hg-primary-static" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.12" />
            <stop offset="20%" stopColor="#0075d6" stopOpacity="0.85" />
            <stop offset="80%" stopColor="#0075d6" stopOpacity="0.85" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.12" />
          </linearGradient>
          <linearGradient id="hg-shift-static" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#418400" stopOpacity="0.35" />
          </linearGradient>
          <radialGradient id="hg-glow-blue-static" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="hg-glow-green-static" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.3" />
            <stop offset="100%" stopColor="#418400" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="hg-glow-gold-static" cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="#fcd400" stopOpacity="0.3" />
            <stop offset="100%" stopColor="#fcd400" stopOpacity="0" />
          </radialGradient>
          <filter id="hg-blur-static">
            <feGaussianBlur in="SourceGraphic" stdDeviation="6" />
          </filter>
        </defs>

        <HeroBackdrop />
        <HeroPaths
          primaryStroke="url(#hg-primary-static)"
          shiftStroke="url(#hg-shift-static)"
          animated={false}
        />

        <g filter="url(#hg-blur-static)">
          <circle cx="50" cy="200" r="28" fill="url(#hg-glow-blue-static)" opacity="0.55" />
          <circle cx="340" cy="65" r="32" fill="url(#hg-glow-green-static)" opacity="0.55" />
          <circle cx="660" cy="335" r="28" fill="url(#hg-glow-gold-static)" opacity="0.5" />
          <circle cx="870" cy="200" r="32" fill="url(#hg-glow-blue-static)" opacity="0.45" />
        </g>

        <circle cx="50" cy="200" r="6.5" fill="#0075d6" />
        <circle cx="340" cy="65" r="7.5" fill="#418400" />
        <circle cx="500" cy="200" r="5" fill="#0075d6" opacity="0.72" />
        <circle cx="660" cy="335" r="7.5" fill="#fcd400" />
        <circle cx="870" cy="200" r="8.5" fill="#1b1b1b" />

        <HeroSecondaryNodes />
        <HeroAccents />
      </svg>
    </div>
  );
}

function HeroBackdrop() {
  return (
    <>
      <g opacity="0.035">
        {Array.from({ length: 8 }, (_, row) =>
          Array.from({ length: 18 }, (_, col) => (
            <circle
              key={`d${row}-${col}`}
              cx={col * 52 + 16}
              cy={row * 44 + 15}
              r="1.2"
              fill="#1b1b1b"
            />
          ))
        ).flat()}
      </g>

      <g opacity="0.05" stroke="#0075d6" strokeWidth="1" fill="none">
        <path d="M 0,100 Q 225,140 450,100 T 900,120" />
        <path d="M 0,260 Q 225,220 450,260 T 900,240" />
        <path d="M 0,175 Q 300,200 600,170 T 900,185" />
      </g>
    </>
  );
}

function HeroPaths({
  primaryStroke = 'url(#hg-primary)',
  shiftStroke,
  animated = true,
}: {
  primaryStroke?: string;
  shiftStroke: string;
  animated?: boolean;
}) {
  const mainPathClass = animated ? 'hero-draw' : undefined;
  const upperPathClass = animated ? 'hero-draw hero-draw-d1' : undefined;
  const lowerPathClass = animated ? 'hero-draw hero-draw-d2' : undefined;
  const crossPathClass = animated ? 'hero-draw hero-draw-d3' : undefined;
  const secondaryPathClass = animated ? 'hero-draw hero-draw-d2' : undefined;

  return (
    <>
      <path
        className={mainPathClass}
        d="M 50,200 C 170,200 200,65 340,65 S 520,335 660,335 S 840,65 870,200"
        stroke={primaryStroke}
        strokeWidth="2.5"
        strokeLinecap="round"
      />
      <path
        className={upperPathClass}
        d="M 260,120 C 300,45 420,65 480,130"
        stroke="#0075d6"
        strokeWidth="1.5"
        strokeOpacity="0.4"
        strokeLinecap="round"
      />
      <path
        className={lowerPathClass}
        d="M 580,280 C 635,355 740,320 790,250"
        stroke="#418400"
        strokeWidth="1.5"
        strokeOpacity="0.35"
        strokeLinecap="round"
      />
      <path
        className={crossPathClass}
        d="M 340,65 Q 500,200 660,335"
        stroke={shiftStroke}
        strokeWidth="1"
        strokeOpacity="0.22"
        strokeLinecap="round"
        strokeDasharray="5 8"
      />
      <path
        className={secondaryPathClass}
        d="M 480,130 C 510,170 540,200 560,230"
        stroke="#0075d6"
        strokeWidth="1"
        strokeOpacity="0.2"
        strokeLinecap="round"
      />
    </>
  );
}

function HeroSecondaryNodes() {
  return (
    <>
      <circle cx="480" cy="130" r="3.5" fill="#0075d6" opacity="0.5" />
      <circle cx="260" cy="120" r="3" fill="#0075d6" opacity="0.4" />
      <circle cx="580" cy="280" r="3" fill="#418400" opacity="0.4" />
      <circle cx="790" cy="250" r="3.5" fill="#418400" opacity="0.5" />
    </>
  );
}

function HeroAccents({ animated = false }: { animated?: boolean }) {
  return (
    <>
      <g stroke="#c0c7d6" strokeWidth="1" opacity="0.2">
        <line x1="186" y1="170" x2="194" y2="170" />
        <line x1="190" y1="166" x2="190" y2="174" />

        <line x1="406" y1="200" x2="414" y2="200" />
        <line x1="410" y1="196" x2="410" y2="204" />

        <line x1="726" y1="160" x2="734" y2="160" />
        <line x1="730" y1="156" x2="730" y2="164" />

        <line x1="136" y1="280" x2="144" y2="280" />
        <line x1="140" y1="276" x2="140" y2="284" />

        <line x1="616" y1="120" x2="624" y2="120" />
        <line x1="620" y1="116" x2="620" y2="124" />
      </g>

      <circle
        cx="500"
        cy="200"
        r="18"
        fill="none"
        stroke="#0075d6"
        strokeWidth="0.8"
        strokeDasharray="3 5"
        opacity="0.18"
      >
        {animated ? (
          <animateTransform
            attributeName="transform"
            type="rotate"
            from="0 500 200"
            to="360 500 200"
            dur="24s"
            repeatCount="indefinite"
          />
        ) : null}
      </circle>

      <circle
        cx="340"
        cy="65"
        r="20"
        fill="none"
        stroke="#418400"
        strokeWidth="0.8"
        strokeDasharray="4 6"
        opacity="0.14"
      >
        {animated ? (
          <animateTransform
            attributeName="transform"
            type="rotate"
            from="0 340 65"
            to="-360 340 65"
            dur="30s"
            repeatCount="indefinite"
          />
        ) : null}
      </circle>

      <circle
        cx="870"
        cy="200"
        r="16"
        fill="none"
        stroke="#1b1b1b"
        strokeWidth="1.5"
        opacity="0.12"
      />
      <circle
        cx="870"
        cy="200"
        r="22"
        fill="none"
        stroke="#1b1b1b"
        strokeWidth="0.5"
        strokeDasharray="2 4"
        opacity="0.08"
      />
    </>
  );
}
