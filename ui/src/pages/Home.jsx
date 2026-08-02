import { Fragment, useEffect, useRef, useState } from "react";
import { CopyCommand } from "../components/CodeBlock.jsx";
import { RotatingText, TerminalDemo } from "../components/TerminalDemo.jsx";
import { useReducedMotion } from "../hooks/useReducedMotion.js";
import { INSTALL_COMMAND, INSTALL_COMMAND_DISPLAY } from "../constants.js";
import {
  BENTO_FEATURES,
  HERO_TERMINAL_DEMO,
  WORKFLOW_TERMINAL_DEMO,
} from "../data/terminalDemos.js";

function Topology() {
  const ref = useRef(null);
  const [offset, setOffset] = useState(0);
  const reducedMotion = useReducedMotion();

  useEffect(() => {
    if (reducedMotion) {
      return;
    }
    const node = ref.current;
    if (!node) {
      return;
    }
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setOffset(entry.intersectionRatio * 24);
        }
      },
      { threshold: [0, 0.25, 0.5, 0.75, 1] },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [reducedMotion]);

  return (
    <div
      className="topology"
      ref={ref}
      style={
        reducedMotion ? undefined : { transform: `translateY(${offset}px)` }
      }
    >
      <svg
        viewBox="0 0 520 370"
        role="img"
        aria-label="Local Outpost CLI connected to a remote Linux host"
      >
        <defs>
          <pattern
            id="dots"
            width="26"
            height="26"
            patternUnits="userSpaceOnUse"
          >
            <circle cx="2" cy="2" r="1" fill="#60a5fa" opacity=".22" />
          </pattern>
          <filter id="glow">
            <feGaussianBlur stdDeviation="5" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <rect width="520" height="370" fill="url(#dots)" />
        <path
          d="M125 185 C205 110 292 110 360 185 S400 260 430 225"
          fill="none"
          stroke="#2563ff"
          strokeWidth="1.5"
          opacity=".75"
        />
        <path
          d="M125 185 H360"
          stroke="#60a5fa"
          strokeDasharray="4 7"
          opacity=".45"
        />
        {[
          [125, 185],
          [360, 185],
          [430, 225],
        ].map(([cx, cy]) => (
          <Fragment key={cx}>
            <circle cx={cx} cy={cy} r="8" fill="#2563ff" filter="url(#glow)" />
            <circle cx={cx} cy={cy} r="18" fill="none" stroke="#2563ff" />
          </Fragment>
        ))}
        <text x="82" y="238">
          LOCAL
        </text>
        <text x="82" y="258" className="bright">
          outpost CLI
        </text>
        <text x="315" y="145">
          SSH →
        </text>
        <text x="325" y="238">
          REMOTE
        </text>
        <text x="325" y="258" className="bright">
          Linux host
        </text>
        <text x="390" y="285" className="blue">
          services
        </text>
      </svg>
    </div>
  );
}

export function Home() {
  return (
    <>
      <section
        className="hero hero-artistic wrap grid-bg grid-bg-drift"
        id="top"
      >
        <div className="hero-copy">
          <h1 className="hero-title">
            <span>Your laptop stays light.</span>
            <span className="hero-title-accent">The host does the work.</span>
          </h1>
          <p className="lead">
            Outpost turns a remote Linux host into a shared development
            environment you control from your local terminal.
          </p>
          <div className="actions">
            <CopyCommand
              command={INSTALL_COMMAND}
              display={INSTALL_COMMAND_DISPLAY}
            />
            <a className="hero-docs-link" href="#docs">
              Read the docs →
            </a>
          </div>
        </div>
        <div className="hero-terminal-wrap">
          <TerminalDemo script={HERO_TERMINAL_DEMO} />
        </div>
      </section>

      <section id="capabilities" className="light-section bento-section">
        <div className="wrap">
          <div className="bento-intro">
            <div className="eyebrow">Capabilities</div>
            <h2>
              On the host.
              <span className="stacked-line">Not your laptop.</span>
            </h2>
            <p className="bento-lead">
              Run workloads on infrastructure you can inspect, share, migrate,
              and control.
            </p>
          </div>
          <div className="bento-grid">
            {BENTO_FEATURES.map((feature) => (
              <article
                className={`bento-card ${feature.featured ? "featured" : ""}`}
                key={feature.title}
              >
                <div className="bento-icon">{feature.icon}</div>
                <h3>{feature.title}</h3>
                <p>{feature.copy}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section id="how-it-works">
        <div className="wrap architecture">
          <div>
            <div className="eyebrow">The connection</div>
            <h2>
              Local workflow.
              <span className="stacked-line">Remote runtime.</span>
            </h2>
            <p className="arch-copy">
              Outpost connects over SSH, bootstraps missing tools on first use,
              syncs project files, and brings services back to your localhost.
              There is no permanent Outpost agent on the server.
            </p>
            <div className="arch-list">
              <div>
                <strong className="mono">Your terminal</strong>
                <span>Commands, projects, credentials</span>
              </div>
              <div>
                <strong className="mono">SSH connection</strong>
                <span>Controlled path to the host</span>
              </div>
              <div>
                <strong className="mono">Remote host</strong>
                <span>Docker, kind, k3d, Incus, services</span>
              </div>
            </div>
          </div>
          <Topology />
        </div>
      </section>

      <section className="workflow light-section">
        <div className="wrap">
          <div className="section-head">
            <div>
              <div className="eyebrow">Start from where you are</div>
              <h2>
                Bring your VPS. Or provision one on{" "}
                <RotatingText items={["AWS", "Hetzner", "your cloud"]} />.
              </h2>
            </div>
            <p className="section-copy">
              A short path from a repository to a running stack on
              infrastructure you own.
            </p>
          </div>
          <TerminalDemo script={WORKFLOW_TERMINAL_DEMO} />
        </div>
      </section>

      <section className="cta cta-artistic grid-bg">
        <div className="wrap cta-inner">
          <div>
            <div className="eyebrow">&gt;_ Keep your laptop light.</div>
            <h2>
              Develop remotely.
              <span className="stacked-line">Work from anywhere.</span>
            </h2>
          </div>
          <CopyCommand
            command={INSTALL_COMMAND}
            display={INSTALL_COMMAND_DISPLAY}
          />
        </div>
      </section>
    </>
  );
}
