import MiniSearch from "minisearch";
import { pages } from "./docs.js";

function stripMarkdown(body) {
  return body
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/`[^`]+`/g, " ")
    .replace(/^#+\s+/gm, "")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[*_>|]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

const docs = Object.values(pages).map((page) => ({
  id: page.slug,
  title: page.title,
  slug: page.slug,
  section: page.section,
  text: stripMarkdown(page.body),
}));

const miniSearch = new MiniSearch({
  fields: ["title", "text"],
  storeFields: ["title", "slug", "section"],
  searchOptions: {
    boost: { title: 3 },
    fuzzy: 0.2,
    prefix: true,
  },
});

miniSearch.addAll(docs);

export function searchDocs(query, limit = 12) {
  const q = query.trim();
  if (!q) {
    return [];
  }
  return miniSearch.search(q).slice(0, limit);
}

export function docResultHref(slug) {
  return slug === "index" ? "#docs" : `#docs/${slug}`;
}
