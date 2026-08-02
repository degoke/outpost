import { useRef, useState } from "react";

export function CopyCommand({ command, display = command, className = "" }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    navigator.clipboard?.writeText(command).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div
      className={`copy-command ${className}`.trim()}
      title={display === command ? undefined : command}
    >
      <pre className="mono">
        <span className="prompt">$</span> {display}
      </pre>
      <button
        type="button"
        onClick={copy}
        aria-label={`Copy command: ${command}`}
      >
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}

export function CodeBlock({ children, className }) {
  const [copied, setCopied] = useState(false);
  const codeRef = useRef(null);
  const languageMatch = className?.match(/language-(\S+)/);
  const language = languageMatch ? languageMatch[1] : "text";

  const copy = () => {
    const text = codeRef.current?.textContent ?? "";
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div className="docs-code-block">
      <div className="docs-code-header">
        <span className="docs-code-lang mono">{language}</span>
        <button type="button" onClick={copy} aria-label="Copy code">
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre>
        <code ref={codeRef} className={className}>
          {children}
        </code>
      </pre>
    </div>
  );
}
