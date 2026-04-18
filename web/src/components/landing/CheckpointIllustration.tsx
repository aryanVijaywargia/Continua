/**
 * Feature illustration for the "Reliable by default" section.
 *
 * Visualizes the core checkpoint+resume story: a linear workflow with
 * completed checkpoint nodes, a failure point, and an arc returning
 * execution to the most recent checkpoint. Uses SMIL + CSS animation
 * to stay self-contained (no runtime deps) and fully interactive with
 * `prefers-reduced-motion`.
 */
export function CheckpointIllustration() {
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
          <radialGradient id="ci-node-glow" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0%" stopColor="#0075d6" stopOpacity="0.55" />
            <stop offset="100%" stopColor="#0075d6" stopOpacity="0" />
          </radialGradient>
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

        {/* ── Background dot grid ── */}
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
            )),
          )}
        </g>

        {/* ── Labels ── */}
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

        {/* ── Main track ── */}
        <line
          x1="60"
          y1="210"
          x2="540"
          y2="210"
          stroke="url(#ci-track)"
          strokeWidth="3"
          strokeLinecap="round"
        />

        {/* Progress overlay - draws from left to crash */}
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

        {/* ── Step nodes (checkpoints) ── */}
        {[
          { x: 95, label: 'fetch', delay: '0.6s' },
          { x: 215, label: 'parse', delay: '1.0s' },
          { x: 335, label: 'llm', delay: '1.4s' },
        ].map((node) => (
          <g key={node.label}>
            <circle
              cx={node.x}
              cy="210"
              r="28"
              fill="url(#ci-success-glow)"
              filter="url(#ci-blur)"
            />
            <circle
              cx={node.x}
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
                begin={node.delay}
                fill="freeze"
              />
              <animate
                attributeName="r"
                values="14;17;14"
                dur="2.4s"
                begin={`${node.delay}`}
                repeatCount="indefinite"
              />
            </circle>
            {/* Checkmark */}
            <path
              d={`M${node.x - 5},210 L${node.x - 1},214 L${node.x + 6},206`}
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
                begin={`${parseFloat(node.delay) + 0.15}s`}
                fill="freeze"
              />
            </path>
            <text
              x={node.x}
              y="258"
              textAnchor="middle"
              fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
              fontSize="11"
              fontWeight="500"
              fill="#404753"
              opacity="0"
            >
              {node.label}
              <animate
                attributeName="opacity"
                from="0"
                to="1"
                dur="0.3s"
                begin={`${parseFloat(node.delay) + 0.2}s`}
                fill="freeze"
              />
            </text>
          </g>
        ))}

        {/* ── Crash point ── */}
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

        {/* ── Remaining grey nodes (not yet reached) ── */}
        {[{ x: 525, label: 'save' }].map((node) => (
          <g key={node.label}>
            <circle
              cx={node.x}
              cy="210"
              r="14"
              fill="#ffffff"
              stroke="#c0c7d6"
              strokeWidth="2"
              strokeDasharray="3 3"
            />
            <text
              x={node.x}
              y="258"
              textAnchor="middle"
              fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
              fontSize="11"
              fontWeight="500"
              fill="#8e95a4"
            >
              {node.label}
            </text>
          </g>
        ))}

        {/* ── Resume arc: from crash back to last successful checkpoint ── */}
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
        {/* Arrowhead at end of resume arc */}
        <path
          d="M335,196 l-6,-7 m6,7 l-9,2"
          stroke="#0075d6"
          strokeWidth="3"
          strokeLinecap="round"
          strokeLinejoin="round"
          fill="none"
          opacity="0"
        >
          <animate
            attributeName="opacity"
            from="0"
            to="1"
            dur="0.3s"
            begin="4.2s"
            fill="freeze"
          />
        </path>

        {/* ── Flowing data particle along the resume arc ── */}
        <circle r="4" fill="#0075d6" opacity="0">
          <animate
            attributeName="opacity"
            values="0;1;1;0"
            keyTimes="0;0.15;0.85;1"
            dur="1.6s"
            begin="3.2s;4.8s+2s"
            repeatCount="indefinite"
          />
          <animateMotion
            dur="1.6s"
            begin="3.2s;4.8s+2s"
            repeatCount="indefinite"
            path="M430,196 Q430,110 335,196"
            rotate="auto"
          />
        </circle>

        {/* ── Bottom caption: tiny persistent state bar ── */}
        <g transform="translate(60, 380)">
          <rect
            width="480"
            height="40"
            rx="8"
            fill="#ffffff"
            stroke="#c0c7d6"
            strokeOpacity="0.4"
            strokeWidth="1"
          />
          <circle cx="20" cy="20" r="5" fill="#418400" />
          <circle cx="20" cy="20" r="5" fill="#418400" opacity="0.35">
            <animate
              attributeName="r"
              values="5;10;5"
              dur="2s"
              repeatCount="indefinite"
            />
            <animate
              attributeName="opacity"
              values="0.35;0;0.35"
              dur="2s"
              repeatCount="indefinite"
            />
          </circle>
          <text
            x="38"
            y="25"
            fontFamily="IBM Plex Mono, SFMono-Regular, monospace"
            fontSize="12"
            fontWeight="500"
            fill="#404753"
          >
            state persisted · checkpoint llm · 3/4 steps
          </text>
        </g>
      </svg>
    </div>
  );
}
