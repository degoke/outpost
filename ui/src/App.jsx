import React, { useEffect, useState } from "react";
import { Footer, Header } from "./components/Layout.jsx";
import { isDocsHash, normalizeHash, slugFromHash } from "./lib/docs.js";
import { Docs } from "./pages/Docs.jsx";
import { Home } from "./pages/Home.jsx";

function routeFromHash(hash) {
  if (isDocsHash(hash)) {
    return { page: "docs", slug: slugFromHash(hash) };
  }
  return { page: "home", slug: "index" };
}

function initialRoute() {
  if (typeof window === "undefined") {
    return routeFromHash("");
  }
  const hash = window.location.hash;
  const normalized = normalizeHash(hash);
  if (normalized !== hash) {
    window.history.replaceState(null, "", normalized);
  }
  return routeFromHash(normalized || hash);
}

export function App() {
  const [route, setRoute] = useState(initialRoute);

  useEffect(() => {
    const onHash = () => {
      const hash = window.location.hash;
      const normalized = normalizeHash(hash);
      if (normalized !== hash) {
        window.history.replaceState(null, "", normalized);
        setRoute(routeFromHash(normalized));
        return;
      }
      setRoute(routeFromHash(hash));
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  return (
    <>
      <Header docsActive={route.page === "docs"} />
      {route.page === "docs" ? (
        <Docs initialSlug={route.slug} />
      ) : (
        <Home />
      )}
      <Footer />
    </>
  );
}
