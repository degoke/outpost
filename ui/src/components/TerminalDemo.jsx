import { Fragment, useCallback, useEffect, useRef, useState } from "react";
import { useReducedMotion } from "../hooks/useReducedMotion.js";

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

function TerminalBar() {
  return (
    <div className="terminal-bar">
      <i />
      <i />
      <i />
      <span className="terminal-title mono">outpost · remote session</span>
      <span className="terminal-state mono">
        <b /> connected
      </span>
    </div>
  );
}

function TerminalChrome({ bodyRef, ariaLive, children }) {
  return (
    <div
      className="terminal terminal-demo"
      {...(ariaLive ? { "aria-live": "polite" } : {})}
    >
      <TerminalBar />
      <div className="terminal-body mono" ref={bodyRef}>
        {children}
      </div>
    </div>
  );
}

export function TerminalDemo({ script, loopDelay = 2400 }) {
  const reducedMotion = useReducedMotion();
  const [history, setHistory] = useState([]);
  const [typing, setTyping] = useState("");
  const timers = useRef([]);
  const bodyRef = useRef(null);

  const clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);

  const schedule = useCallback((fn, ms) => {
    timers.current.push(setTimeout(fn, ms));
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
    if (reducedMotion) {
      return;
    }

    run();
    return clearTimers;
  }, [clearTimers, reducedMotion, run]);

  useEffect(() => {
    const body = bodyRef.current;
    if (!body) {
      return;
    }
    body.scrollTop = body.scrollHeight;
  }, [history, typing]);

  if (reducedMotion) {
    return (
      <TerminalChrome bodyRef={bodyRef}>
        {script.map((block, blockIndex) => (
          <Fragment key={block.command}>
            <div>
              <span className="prompt">$</span> {block.command}
            </div>
            {block.lines.map((line, lineIndex) => (
              <TerminalLine key={lineIndex} line={line} />
            ))}
            {blockIndex < script.length - 1 ? (
              <div className="terminal-blank" />
            ) : null}
          </Fragment>
        ))}
      </TerminalChrome>
    );
  }

  return (
    <TerminalChrome bodyRef={bodyRef} ariaLive>
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
    </TerminalChrome>
  );
}

export function RotatingText({ items, interval = 2600 }) {
  const reducedMotion = useReducedMotion();
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
