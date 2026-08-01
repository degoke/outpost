import React, { useEffect, useMemo, useRef, useState } from "react";
import { GITHUB } from "../constants.js";
import { DocsSearch } from "../components/DocsSearch.jsx";
import { Markdown } from "../components/Markdown.jsx";
import {
  getDocNav,
  getDocPage,
  getDocPosition,
  headingId,
  slugFromHash,
} from "../lib/docs.js";

function extractHeadings(body) {
  const headings = [];
  for (const line of body.split("\n")) {
    const match = line.match(/^(#{2,3})\s+(.+)$/);
    if (match) {
      const level = match[1].length;
      const text = match[2].replace(/`/g, "");
      headings.push({ level, text, id: headingId(text) });
    }
  }
  return headings;
}

function PagerLink({ item, direction }) {
  if (!item) {
    return <span />;
  }
  return (
    <a
      className={`docs-pager-link ${direction}`}
      href={item.slug === "index" ? "#docs" : `#docs/${item.slug}`}
    >
      <span className="docs-pager-label mono">
        {direction === "prev" ? "← Previous" : "Next →"}
      </span>
      <span className="docs-pager-title">{item.title}</span>
    </a>
  );
}

export function Docs({ initialSlug }) {
  const [slug, setSlug] = useState(initialSlug || "index");
  const [navOpen, setNavOpen] = useState(false);
  const [activeHeading, setActiveHeading] = useState(null);
  const articleRef = useRef(null);
  const nav = useMemo(() => getDocNav(), []);
  const page = useMemo(() => getDocPage(slug), [slug]);
  const headings = useMemo(() => extractHeadings(page?.body || ""), [page]);
  const { sectionTitle, prev, next } = useMemo(
    () => getDocPosition(page?.slug),
    [page],
  );

  useEffect(() => {
    const onHash = () => setSlug(slugFromHash(window.location.hash));
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    window.scrollTo(0, 0);
  }, [slug]);

  useEffect(() => {
    if (headings.length === 0) {
      setActiveHeading(null);
      return;
    }
    const elements = headings
      .map((h) => document.getElementById(h.id))
      .filter(Boolean);
    if (elements.length === 0) {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible.length > 0) {
          setActiveHeading(visible[0].target.id);
        }
      },
      { rootMargin: "-88px 0px -70% 0px", threshold: 0 },
    );
    elements.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, [headings, page]);

  const selectPage = (nextSlug) => {
    setSlug(nextSlug);
    setNavOpen(false);
    const hash = nextSlug === "index" ? "#docs" : `#docs/${nextSlug}`;
    window.history.replaceState(null, "", hash);
  };

  const editHref = `${GITHUB}/edit/main/docs/${page.path}.md`;

  return (
    <main className="docs-page" id="docs">
      <div className="wrap docs-shell">
        <button
          type="button"
          className="docs-nav-toggle mono"
          onClick={() => setNavOpen((open) => !open)}
          aria-expanded={navOpen}
        >
          {navOpen ? "Close menu" : "Browse docs"}
        </button>

        <aside className={`docs-nav ${navOpen ? "open" : ""}`}>
          <div className="docs-nav-search">
            <DocsSearch onSelect={selectPage} compact />
          </div>
          {nav.map((section) => (
            <div key={section.id} className="docs-nav-section">
              <div className="mono docs-nav-label">{section.title}</div>
              {section.items.map((item) => (
                <a
                  key={item.slug}
                  href={item.slug === "index" ? "#docs" : `#docs/${item.slug}`}
                  className={item.slug === slug ? "selected" : ""}
                  onClick={(e) => {
                    e.preventDefault();
                    selectPage(item.slug);
                  }}
                >
                  {item.title}
                </a>
              ))}
            </div>
          ))}
        </aside>

        <div className="docs-main">
          <div className="docs-breadcrumb mono">
            <a href="#docs">Docs</a>
            {sectionTitle ? (
              <>
                <span className="docs-breadcrumb-sep">/</span>
                <span>{sectionTitle}</span>
              </>
            ) : null}
            <span className="docs-breadcrumb-sep">/</span>
            <span className="docs-breadcrumb-current">{page.title}</span>
          </div>

          <article className="docs-article" ref={articleRef}>
            <Markdown key={page.slug}>{page.body}</Markdown>
          </article>

          <div className="docs-page-footer">
            <a
              className="docs-edit-link"
              href={editHref}
              target="_blank"
              rel="noopener noreferrer"
            >
              Edit this page on GitHub ↗
            </a>
            {(prev || next) && (
              <nav className="docs-pager" aria-label="Page navigation">
                <PagerLink item={prev} direction="prev" />
                <PagerLink item={next} direction="next" />
              </nav>
            )}
          </div>
        </div>

        {headings.length > 0 ? (
          <aside className="docs-toc" aria-label="On this page">
            <div className="mono docs-nav-label">On this page</div>
            {headings.map((h) => (
              <a
                key={h.id}
                href={`#${h.id}`}
                className={`${h.level === 3 ? "depth-2" : ""} ${
                  activeHeading === h.id ? "active" : ""
                }`.trim()}
              >
                {h.text}
              </a>
            ))}
          </aside>
        ) : null}
      </div>
    </main>
  );
}
