import { useMediaQuery } from '../../hooks/useMediaQuery';

export function CheckpointIllustration() {
  const prefersReducedMotion = useMediaQuery(
    '(prefers-reduced-motion: reduce)'
  );

  return prefersReducedMotion ? (
    <StaticCheckpointIllustration />
  ) : (
    <AnimatedCheckpointIllustration />
  );
}

function AnimatedCheckpointIllustration() {
  return (
    <div className="relative aspect-[4/3] overflow-hidden rounded-xl border border-outline-variant/30 bg-gradient-to-br from-[#f4f7fc] via-[#eef3fb] to-[#e4ecf8] dark:from-[#0f1624] dark:via-[#111a2e] dark:to-[#0a1020]">
      <svg
        viewBox="0 0 600 450"
        className="h-full w-full"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="ci-track" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.2" />
            <stop offset="50%" stopColor="#0075d6" stopOpacity="0.9" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.2" />
          </linearGradient>
          <linearGradient id="ci-resume" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.9" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.9" />
          </linearGradient>
          <radialGradient id="ci-success-glow" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.55" />
            <stop offset="100%" stopColor="#418400" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="ci-crash-glow" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0%" stopColor="#ba1a1a" stopOpacity="0.6" />
            <stop offset="100%" stopColor="#ba1a1a" stopOpacity="0" />
          </radialGradient>
          <filter id="ci-blur" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="10" />
          </filter>
        </defs>

        <CheckpointBackdrop />
        <CheckpointLabels />

        <line
          x1="60"
          y1="210"
          x2="540"
          y2="210"
          stroke="url(#ci-track)"
          strokeWidth="3"
          strokeLinecap="round"
        />

        <line
          x1="60"
          y1="210"
          x2="420"
          y2="210"
          stroke="#0075d6"
          strokeWidth="4"
          strokeLinecap="round"
          className="hero-draw"
          strokeDasharray="360"
          strokeDashoffset="360"
        >
          <animate
            attributeName="stroke-dashoffset"
            from="360"
            to="0"
            dur="2.2s"
            begin="0.4s"
            fill="freeze"
            calcMode="spline"
            keySplines="0.22 1 0.36 1"
          />
        </line>

        <AnimatedCheckpointNode x={95} label="fetch" delay="0.6s" />
        <AnimatedCheckpointNode x={215} label="parse" delay="1.0s" />
        <AnimatedCheckpointNode x={335} label="llm" delay="1.4s" />
        <AnimatedCrashNode />
        <QueuedCheckpointNode x={525} label="save" />

        <path
          d="M430,196 Q430,110 335,196"
          stroke="url(#ci-resume)"
          strokeWidth="3"
          strokeLinecap="round"
          fill="none"
          strokeDasharray="260"
          strokeDashoffset="260"
          opacity="0.9"
        >
          <animate
            attributeName="stroke-dashoffset"
            from="260"
            to="0"
            dur="1.1s"
            begin="3.2s"
            fill="freeze"
            calcMode="spline"
            keySplines="0.22 1 0.36 1"
          />
        </path>
        <path
          d="M335,196 l-6,-7 m6,7 l-9,2"
          stroke="#0075d6"
          strokeWidth="3"
          strokeLinecap="round"
          strokeLinejoin="round"
          fill="none"
        >
          <animate
            attributeName="opacity"
            from="0"
            to="1"
            dur="0.25s"
            begin="4.1s"
            fill="freeze"
          />
        </path>
      </svg>
    </div>
  );
}

function StaticCheckpointIllustration() {
  return (
    <div className="relative aspect-[4/3] overflow-hidden rounded-xl border border-outline-variant/30 bg-gradient-to-br from-[#f4f7fc] via-[#eef3fb] to-[#e4ecf8] dark:from-[#0f1624] dark:via-[#111a2e] dark:to-[#0a1020]">
      <svg
        viewBox="0 0 600 450"
        className="h-full w-full"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="ci-track-static" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.2" />
            <stop offset="50%" stopColor="#0075d6" stopOpacity="0.9" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.2" />
          </linearGradient>
          <linearGradient id="ci-resume-static" x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.9" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0.9" />
          </linearGradient>
          <radialGradient id="ci-success-glow-static" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0%" stopColor="#418400" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#418400" stopOpacity="0" />
          </radialGradient>
          <radialGradient id="ci-crash-glow-static" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0%" stopColor="#ba1a1a" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#ba1a1a" stopOpacity="0" />
          </radialGradient>
          <filter id="ci-blur-static" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="10" />
          </filter>
        </defs>

        <CheckpointBackdrop />
        <CheckpointLabels />

        <line
          x1="60"
          y1="210"
          x2="540"
          y2="210"
          stroke="url(#ci-track-static)"
          strokeWidth="3"
          strokeLinecap="round"
        />
        <line
          x1="60"
          y1="210"
          x2="420"
          y2="210"
          stroke="#0075d6"
          strokeWidth="4"
          strokeLinecap="round"
        />

        <StaticCheckpointNode x={95} label="fetch" />
        <StaticCheckpointNode x={215} label="parse" />
        <StaticCheckpointNode x={335} label="llm" />

        <g>
          <circle
            cx="430"
            cy="210"
            r="34"
            fill="url(#ci-crash-glow-static)"
            filter="url(#ci-blur-static)"
            opacity="0.7"
          />
          <circle
            cx="430"
            cy="210"
            r="14"
            fill="#ffffff"
            stroke="#ba1a1a"
            strokeWidth="2.5"
          />
          <path
            d="M425,205 L435,215 M435,205 L425,215"
            stroke="#ba1a1a"
            strokeWidth="2.5"
            strokeLinecap="round"
          />
          <text
            x="430"
            y="258"
            textAnchor="middle"
            fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
            fontSize="11"
            fontWeight="600"
            fill="#ba1a1a"
          >
            CRASH
          </text>
        </g>

        <QueuedCheckpointNode x={525} label="save" />

        <path
          d="M430,196 Q430,110 335,196"
          stroke="url(#ci-resume-static)"
          strokeWidth="3"
          strokeLinecap="round"
          fill="none"
          opacity="0.9"
        />
        <path
          d="M335,196 l-6,-7 m6,7 l-9,2"
          stroke="#0075d6"
          strokeWidth="3"
          strokeLinecap="round"
          strokeLinejoin="round"
          fill="none"
        />
      </svg>
    </div>
  );
}

function CheckpointBackdrop() {
  return (
    <g opacity="0.12">
      {Array.from({ length: 12 }).map((_, row) =>
        Array.from({ length: 16 }).map((_, col) => (
          <circle
            key={`${row}-${col}`}
            cx={40 + col * 35}
            cy={40 + row * 32}
            r="1.4"
            fill="#1b1b1b"
          />
        ))
      )}
    </g>
  );
}

function CheckpointLabels() {
  return (
    <>
      <text
        x="50"
        y="80"
        fontFamily="Inter, system-ui, sans-serif"
        fontSize="11"
        fontWeight="700"
        letterSpacing="2"
        fill="#707785"
      >
        WORKFLOW
      </text>
      <text
        x="50"
        y="350"
        fontFamily="Inter, system-ui, sans-serif"
        fontSize="11"
        fontWeight="700"
        letterSpacing="2"
        fill="#707785"
      >
        RESUME FROM LAST CHECKPOINT
      </text>
    </>
  );
}

function AnimatedCheckpointNode({
  x,
  label,
  delay,
}: {
  x: number;
  label: string;
  delay: string;
}) {
  return (
    <g>
      <circle
        cx={x}
        cy="210"
        r="28"
        fill="url(#ci-success-glow)"
        filter="url(#ci-blur)"
      />
      <circle
        cx={x}
        cy="210"
        r="14"
        fill="#ffffff"
        stroke="#418400"
        strokeWidth="2.5"
        opacity="0"
      >
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.4s"
          begin={delay}
          fill="freeze"
        />
        <animate
          attributeName="r"
          values="14;17;14"
          dur="2.4s"
          begin={delay}
          repeatCount="indefinite"
        />
      </circle>
      <path
        d={`M${x - 5},210 L${x - 1},214 L${x + 6},206`}
        stroke="#418400"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity="0"
        fill="none"
      >
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.3s"
          begin={`${parseFloat(delay) + 0.15}s`}
          fill="freeze"
        />
      </path>
      <text
        x={x}
        y="258"
        textAnchor="middle"
        fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
        fontSize="11"
        fontWeight="500"
        fill="#404753"
        opacity="0"
      >
        {label}
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.3s"
          begin={`${parseFloat(delay) + 0.2}s`}
          fill="freeze"
        />
      </text>
    </g>
  );
}

function StaticCheckpointNode({ x, label }: { x: number; label: string }) {
  return (
    <g>
      <circle
        cx={x}
        cy="210"
        r="28"
        fill="url(#ci-success-glow-static)"
        filter="url(#ci-blur-static)"
      />
      <circle
        cx={x}
        cy="210"
        r="14"
        fill="#ffffff"
        stroke="#418400"
        strokeWidth="2.5"
      />
      <path
        d={`M${x - 5},210 L${x - 1},214 L${x + 6},206`}
        stroke="#418400"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
      <text
        x={x}
        y="258"
        textAnchor="middle"
        fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
        fontSize="11"
        fontWeight="500"
        fill="#404753"
      >
        {label}
      </text>
    </g>
  );
}

function AnimatedCrashNode() {
  return (
    <g>
      <circle
        cx="430"
        cy="210"
        r="34"
        fill="url(#ci-crash-glow)"
        filter="url(#ci-blur)"
        opacity="0"
      >
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.4s"
          begin="2.6s"
          fill="freeze"
        />
        <animate
          attributeName="r"
          values="34;42;34"
          dur="2s"
          begin="2.6s"
          repeatCount="indefinite"
        />
      </circle>
      <circle
        cx="430"
        cy="210"
        r="14"
        fill="#ffffff"
        stroke="#ba1a1a"
        strokeWidth="2.5"
        opacity="0"
      >
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.3s"
          begin="2.6s"
          fill="freeze"
        />
      </circle>
      <path
        d="M425,205 L435,215 M435,205 L425,215"
        stroke="#ba1a1a"
        strokeWidth="2.5"
        strokeLinecap="round"
        opacity="0"
      >
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.3s"
          begin="2.75s"
          fill="freeze"
        />
      </path>
      <text
        x="430"
        y="258"
        textAnchor="middle"
        fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
        fontSize="11"
        fontWeight="600"
        fill="#ba1a1a"
        opacity="0"
      >
        CRASH
        <animate
          attributeName="opacity"
          from="0"
          to="1"
          dur="0.3s"
          begin="2.8s"
          fill="freeze"
        />
      </text>
    </g>
  );
}

function QueuedCheckpointNode({ x, label }: { x: number; label: string }) {
  return (
    <g>
      <circle
        cx={x}
        cy="210"
        r="14"
        fill="#ffffff"
        stroke="#c0c7d6"
        strokeWidth="2"
        strokeDasharray="3 3"
      />
      <text
        x={x}
        y="258"
        textAnchor="middle"
        fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
        fontSize="11"
        fontWeight="500"
        fill="#8e95a4"
      >
        {label}
      </text>
    </g>
  );
}
