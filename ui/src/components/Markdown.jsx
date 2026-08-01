import React from "react";
import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import { CodeBlock } from "./CodeBlock.jsx";
import { docHref, headingId } from "../lib/docs.js";

function MarkdownH1({ children }) {
  const id = headingId(String(children));
  return <h1 id={id}>{children}</h1>;
}

function MarkdownH2({ children }) {
  const id = headingId(String(children));
  return <h2 id={id}>{children}</h2>;
}

function MarkdownH3({ children }) {
  const id = headingId(String(children));
  return <h3 id={id}>{children}</h3>;
}

export function Markdown({ children }) {
  return (
    <div className="docs-content">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeHighlight]}
        components={{
          h1: MarkdownH1,
          h2: MarkdownH2,
          h3: MarkdownH3,
          a({ href, children }) {
            const resolved = docHref(href);
            const external = resolved?.startsWith("http");
            return (
              <a
                href={resolved}
                target={external ? "_blank" : undefined}
                rel={external ? "noopener noreferrer" : undefined}
              >
                {children}
              </a>
            );
          },
          pre({ children }) {
            return <>{children}</>;
          },
          code({ className, children, ...props }) {
            const isInline = !className;
            if (isInline) {
              return (
                <code className="docs-inline-code" {...props}>
                  {children}
                </code>
              );
            }
            return (
              <CodeBlock className={className}>{children}</CodeBlock>
            );
          },
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
