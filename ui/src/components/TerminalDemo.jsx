import React, { useCallback, useEffect, useRef, useState } from "react";

function TerminalLine({ line }) {
  if (line.type === "forward") {
    return (
      <div>
        <span className="route">→</span> {line.from}{" "}
        <span className="dim">→</span> {line.to}
      </div>
    );
  }

  return (
    <div>
      {line.prefix ? (
        <>
          <span className={line.className}>{line.prefix}</span> {line.text}
        </>
      ) : (
        <span className={line.className}>{line.text}</span>
      )}
    </div>
  );
}

export function TerminalDemo({ script, loopDelay = 2400, whenVisible = false }) {
  const reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const [history, setHistory] = useState([]);
  const [typing, setTyping] = useState("");
  const [inView, setInView] = useState(!whenVisible);
  const timers = useRef([]);
  const rootRef = useRef(null);
  const bodyRef = useRef(null);

  const clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);

  const schedule = useCallback((fn, ms) => {
    const id = setTimeout(fn, ms);
    timers.current.push(id);
    return id;
  }, []);

  const run = useCallback(() => {
    clearTimers();
    setHistory([]);
    setTyping("");

    let delay = 0;
    const typeSpeed = 42;
    const linePause = 320;
    const blockPause = 700;

    script.forEach((block, blockIndex) => {
      for (let i = 0; i < block.command.length; i++) {
        const text = block.command.slice(0, i + 1);
        schedule(() => setTyping(text), delay);
        delay += typeSpeed;
      }

      schedule(() => {
        setHistory((current) => [
          ...current,
          { kind: "command", text: block.command },
        ]);
        setTyping("");
      }, delay + 280);
      delay += 280;

      block.lines.forEach((line) => {
        schedule(() => {
          setHistory((current) => [...current, { kind: "line", line }]);
        }, delay);
        delay += linePause;
      });

      if (blockIndex < script.length - 1) {
        schedule(() => {
          setHistory((current) => [...current, { kind: "blank" }]);
        }, delay);
        delay += blockPause;
      }
    });

    schedule(run, delay + loopDelay);
  }, [clearTimers, loopDelay, schedule, script]);

  useEffect(() => {
    if (!whenVisible || reducedMotion) {
      return;
    }

    const node = rootRef.current;
    if (!node) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => setInView(entry.isIntersecting),
      { threshold: 0.35 },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [reducedMotion, whenVisible]);

  useEffect(() => {
    if (!inView) {
      clearTimers();
      setHistory([]);
      setTyping("");
      return;
    }

    if (reducedMotion) {
      return;
    }

    run();
    return clearTimers;
  }, [clearTimers, inView, reducedMotion, run]);

  useEffect(() => {
    const body = bodyRef.current;
    if (!body) {
      return;
    }
    body.scrollTop = body.scrollHeight;
  }, [history, typing]);

  if (reducedMotion) {
    return (
      <div className="terminal terminal-demo" ref={rootRef}>
        <div className="terminal-bar">
          <i />
          <i />
          <i />
          <span className="terminal-title mono">outpost · remote session</span>
          <span className="terminal-state mono">
            <b /> connected
          </span>
        </div>
        <div className="terminal-body mono" ref={bodyRef}>
          {script.map((block, blockIndex) => (
            <React.Fragment key={block.command}>
              <div>
                <span className="prompt">$</span> {block.command}
              </div>
              {block.lines.map((line, lineIndex) => (
                <TerminalLine key={lineIndex} line={line} />
              ))}
              {blockIndex < script.length - 1 ? (
                <div className="terminal-blank" />
              ) : null}
            </React.Fragment>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="terminal terminal-demo" aria-live="polite" ref={rootRef}>
      <div className="terminal-bar">
        <i />
        <i />
        <i />
        <span className="terminal-title mono">outpost · remote session</span>
        <span className="terminal-state mono">
          <b /> connected
        </span>
      </div>
      <div className="terminal-body mono" ref={bodyRef}>
        {history.map((entry, index) => {
          if (entry.kind === "command") {
            return (
              <div key={index}>
                <span className="prompt">$</span> {entry.text}
              </div>
            );
          }
          if (entry.kind === "blank") {
            return <div key={index} className="terminal-blank" />;
          }
          return <TerminalLine key={index} line={entry.line} />;
        })}
        {typing ? (
          <div>
            <span className="prompt">$</span> {typing}
            <span className="terminal-cursor" aria-hidden="true" />
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function CommandTicker({ commands }) {
  const reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const items = [...commands, ...commands];

  return (
    <div className="command-ticker" aria-hidden="true">
      <div className={`command-ticker-track ${reducedMotion ? "static" : ""}`}>
        {items.map((cmd, i) => (
          <span key={`${cmd}-${i}`} className="mono">
            {cmd}
          </span>
        ))}
      </div>
    </div>
  );
}

export function RotatingText({ items, interval = 2600 }) {
  const reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const [index, setIndex] = useState(0);
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    if (reducedMotion || items.length < 2) {
      return;
    }

    const timer = window.setInterval(() => {
      setVisible(false);
      window.setTimeout(() => {
        setIndex((current) => (current + 1) % items.length);
        setVisible(true);
      }, 220);
    }, interval);

    return () => window.clearInterval(timer);
  }, [interval, items.length, reducedMotion]);

  return (
    <span className="rotating-text-wrap" aria-live="polite">
      <span className={`rotating-text ${visible ? "is-visible" : ""}`}>
        {items[index]}
      </span>
    </span>
  );
}
