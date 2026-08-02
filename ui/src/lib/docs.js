import meta from "../../../docs/meta.json";

const rawModules = import.meta.glob("../../../docs/**/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
});

function parseFrontmatter(raw) {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!match) {
    return { frontmatter: {}, body: raw };
  }
  const frontmatter = {};
  for (const line of match[1].split("\n")) {
    const idx = line.indexOf(":");
    if (idx > 0) {
      const key = line.slice(0, idx).trim();
      let value = line.slice(idx + 1).trim();
      if (value === "true") value = true;
      if (value === "false") value = false;
      if (/^\d+$/.test(value)) value = Number(value);
      frontmatter[key] = value;
    }
  }
  return { frontmatter, body: match[2] };
}

function buildPages() {
  const pages = {};
  for (const [path, raw] of Object.entries(rawModules)) {
    const rel = path.replace(/^.*\/docs\//, "").replace(/\.md$/, "");
    const { frontmatter, body } = parseFrontmatter(raw);
    const slug = frontmatter.slug || rel;
    pages[slug] = {
      slug,
      path: rel,
      title: frontmatter.title || rel,
      section: frontmatter.section || "overview",
      order: frontmatter.order || 0,
      body,
    };
  }
  return pages;
}

const pages = buildPages();

export function getDocNav() {
  return meta.sections.map((section) => ({
    ...section,
    items: section.pages
      .map((id) => pages[id])
      .filter(Boolean)
      .sort((a, b) => a.order - b.order),
  }));
}

export function getDocPage(slug) {
  if (!slug || slug === "index") {
    return pages.index || Object.values(pages)[0];
  }
  return pages[slug] || pages.index;
}

/** Flattened, ordered list of nav pages annotated with their section title. */
function getFlatNavItems() {
  const flat = [];
  for (const section of getDocNav()) {
    for (const item of section.items) {
      flat.push({ ...item, sectionTitle: section.title });
    }
  }
  return flat;
}

/** Section title + adjacent pages for a given slug, used for breadcrumbs and pagers. */
export function getDocPosition(slug) {
  const flat = getFlatNavItems();
  const index = flat.findIndex((item) => item.slug === slug);
  if (index === -1) {
    return { sectionTitle: null, prev: null, next: null };
  }
  return {
    sectionTitle: flat[index].sectionTitle,
    prev: index > 0 ? flat[index - 1] : null,
    next: index < flat.length - 1 ? flat[index + 1] : null,
  };
}

export function docHref(href) {
  if (
    !href ||
    href.startsWith("http") ||
    href.startsWith("#") ||
    href.startsWith("mailto:")
  ) {
    return href;
  }
  if (href === "index" || href === "../index") {
    return "#docs";
  }
  if (href.startsWith("../")) {
    return `#docs/${href.slice(3)}`;
  }
  if (href.includes("/")) {
    return `#docs/${href}`;
  }
  if (pages[href]) {
    return href === "index" ? "#docs" : `#docs/${href}`;
  }
  if (pages[`commands/${href}`]) {
    return `#docs/commands/${href}`;
  }
  if (pages[`guides/${href}`]) {
    return `#docs/guides/${href}`;
  }
  return `#docs/${href}`;
}

export function slugFromHash(hash) {
  if (!hash || hash === "#docs") {
    return "index";
  }
  if (hash.startsWith("#docs/")) {
    return hash.slice(6);
  }
  return "index";
}

export function isDocsHash(hash) {
  return hash === "#docs" || hash.startsWith("#docs/");
}

/** Slugify heading text for in-page anchor links. */
export function headingId(text) {
  return String(text)
    .toLowerCase()
    .replace(/[^\w\s-]/g, "")
    .replace(/\s+/g, "-");
}

const LEGACY_DOC_HASH = {
  "#usage": "#docs",
  "#getting-started": "#docs/getting-started",
  "#hosts": "#docs/commands/host",
  "#projects": "#docs/projects",
  "#connections": "#docs/commands/open",
  "#sharing": "#docs/guides/sharing",
  "#clusters": "#docs/guides/kubernetes",
  "#machines": "#docs/commands/machine",
  "#monitoring": "#docs/commands/monitoring",
  "#reference": "#docs",
  "#options": "#docs/commands/configuration",
};

export function normalizeHash(hash) {
  if (!hash) {
    return hash;
  }
  return LEGACY_DOC_HASH[hash] || hash;
}

export { pages };
