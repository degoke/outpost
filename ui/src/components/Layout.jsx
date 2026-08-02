import { useEffect, useState } from "react";
import { GITHUB } from "../constants.js";

export function Logo() {
  return (
    <a className="logo" href="#top" aria-label="Outpost home">
      <img
        className="logo-image"
        src="/logo-dark.svg"
        alt="outpost"
        width={209}
        height={48}
      />
    </a>
  );
}

export function GitHubStar() {
  const [stars, setStars] = useState(null);

  useEffect(() => {
    fetch("https://api.github.com/repos/degoke/outpost")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => data && setStars(data.stargazers_count))
      .catch(() => {});
  }, []);

  return (
    <a
      className="github-star"
      href={GITHUB}
      target="_blank"
      rel="noopener noreferrer"
      aria-label="Star Outpost on GitHub"
    >
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.75.75 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z" />
      </svg>
      <span>Star</span>
      {stars != null && (
        <span className="github-star-count">{stars.toLocaleString()}</span>
      )}
    </a>
  );
}

export function Header({ docsActive }) {
  return (
    <header>
      <nav className="wrap">
        <Logo />
        <div className="nav-links">
          <a className={docsActive ? "active" : ""} href="#docs">
            Documentation
          </a>
          <GitHubStar />
        </div>
      </nav>
    </header>
  );
}

export function Footer() {
  return (
    <footer>
      <div className="wrap">
        <span>© 2026 Outpost · Remote power. Local control.</span>
        <span>
          <a href={GITHUB}>GitHub</a> · <a href="#docs">Docs</a> ·{" "}
          <a href="#capabilities">Features</a>
        </span>
      </div>
    </footer>
  );
}
