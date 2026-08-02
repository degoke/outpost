import { useEffect, useRef, useState } from "react";
import { searchDocs } from "../lib/search.js";

export function DocsSearch({ onSelect }) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const rootRef = useRef(null);
  const results = query.trim() ? searchDocs(query) : [];

  useEffect(() => {
    const onDocClick = (event) => {
      if (!rootRef.current?.contains(event.target)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, []);

  useEffect(() => {
    setActive(0);
  }, [query]);

  const choose = (slug) => {
    setQuery("");
    setOpen(false);
    onSelect(slug);
  };

  const onKeyDown = (event) => {
    if (!open || results.length === 0) {
      if (event.key === "Escape") {
        setOpen(false);
      }
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((index) => (index + 1) % results.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((index) => (index - 1 + results.length) % results.length);
    } else if (event.key === "Enter") {
      event.preventDefault();
      choose(results[active].slug);
    } else if (event.key === "Escape") {
      setOpen(false);
    }
  };

  return (
    <div className="docs-search" ref={rootRef}>
      <input
        id="docs-search-input"
        type="search"
        className="docs-search-input mono"
        placeholder="Search…"
        value={query}
        autoComplete="off"
        spellCheck={false}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
      />
      {open && query.trim() ? (
        <div className="docs-search-results" role="listbox">
          {results.length === 0 ? (
            <div className="docs-search-empty">No matches for “{query}”</div>
          ) : (
            results.map((result, index) => (
              <button
                key={result.id}
                type="button"
                role="option"
                aria-selected={index === active}
                className={index === active ? "active" : ""}
                onMouseEnter={() => setActive(index)}
                onClick={() => choose(result.slug)}
              >
                <span className="docs-search-title">{result.title}</span>
                <span className="docs-search-meta mono">{result.section}</span>
              </button>
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}
