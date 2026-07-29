import React, { useCallback, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

const GITHUB = "https://github.com/degoke/outpost";
const INSTALL_COMMAND =
  "curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | bash";
const INSTALL_COMMAND_DISPLAY = "curl -fsSL …/install.sh | bash";
const HERO_TERMINAL_DEMO = [
  {
    command: "outpost compose up -d",
    lines: [
      {
        className: "route",
        prefix: "→",
        text: "Syncing project to host dev…",
      },
      { className: "route", prefix: "→", text: "Uploading compose.yaml" },
      { className: "ok", prefix: "✓", text: "Starting services…" },
    ],
  },
  {
    command: "outpost connect",
    lines: [
      { className: "ok", prefix: "✓", text: "Connected to dev over SSH" },
      { type: "forward", from: "127.0.0.1:8080", to: "api:8080" },
      { className: "dim", text: "Press Ctrl+C to stop forwarding." },
    ],
  },
];
const WORKFLOW_TERMINAL_DEMO = [
  {
    command:
      "outpost provider login aws --profile my-profile --region eu-west-1",
    lines: [
      {
        className: "ok",
        prefix: "✓",
        text: "AWS login OK (account 123456789012, region eu-west-1)",
      },
      {
        className: "route",
        prefix: "→",
        text: "ARN: arn:aws:iam::123456789012:user/my-profile",
      },
    ],
  },
  {
    command: "outpost host create dev --provider aws --region eu-west-1",
    lines: [
      {
        className: "route",
        prefix: "→",
        text: "Creating EC2 instance in eu-west-1...",
      },
      {
        className: "route",
        prefix: "→",
        text: "Waiting for SSH on ec2-12-34-56-78.eu-west-1.compute.amazonaws.com...",
      },
      { className: "route", prefix: "→", text: "Bootstrapping remote host..." },
      {
        className: "ok",
        prefix: "✓",
        text: 'Host "dev" created and ready at ec2-12-34-56-78.eu-west-1.compute.amazonaws.com',
      },
    ],
  },
  {
    command: "outpost init --name my-api",
    lines: [],
  },
  {
    command: "outpost compose up -d",
    lines: [{ className: "ok", prefix: "✓", text: "Stack my-api is running" }],
  },
  {
    command: "outpost connect",
    lines: [{ type: "forward", from: "http://127.0.0.1:8080", to: "api:8080" }],
  },
];
const groups = [
  [
    "getting-started",
    "Getting started",
    "Install the CLI, connect a host, initialize a project, and start a remote stack.",
    [
      [
        "Install the CLI",
        [
          [
            "curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | bash",
            "Install on macOS or Linux",
          ],
          [
            "go install github.com/degoke/outpost/cmd/outpost@latest",
            "Install from source with Go 1.26+",
          ],
          ["outpost --help", "Verify the CLI and view commands"],
          [
            'outpost completion zsh > "${fpath[1]}/_outpost"',
            "Enable shell completion",
          ],
        ],
        "Install Outpost locally, verify it, and generate completion for bash, zsh, fish, or powershell.",
      ],
      [
        "Use an existing host",
        [
          [
            "outpost host add dev --hostname 203.0.113.10 --user ubuntu --auth password",
            "Register a host and bootstrap Docker",
          ],
        ],
        "Register a Linux host, verify SSH, and bootstrap Docker.",
      ],
      [
        "Create an AWS host",
        [
          [
            "outpost provider login aws --profile my-profile --region eu-west-1",
            "Authenticate and save your AWS profile",
          ],
          [
            "outpost host create dev --provider aws --region eu-west-1",
            "Provision and register an EC2 host",
          ],
        ],
        "Provision and register an EC2 host.",
      ],
      [
        "Initialize and start",
        [
          ["outpost init --name my-api", "Create project configuration"],
          ["outpost compose up -d", "Start the remote stack"],
          ["outpost connect", "Forward services to localhost"],
        ],
        "Create project configuration, start the stack, and forward ports to localhost.",
      ],
    ],
  ],
  [
    "hosts",
    "Hosts",
    "Register, inspect, select, and manage the machines behind your projects.",
    [
      [
        "Register and inspect",
        [
          [
            "outpost host add dev --hostname 203.0.113.10 --user ubuntu --auth key --identity-file ~/.ssh/vps_key",
            "Add a host with SSH key auth",
          ],
          ["outpost host list", "List registered hosts"],
          ["outpost host verify", "Check connection to the active host"],
          ["outpost host capabilities", "Inspect host capabilities"],
        ],
        "Add a host, list registered hosts, check its connection, and inspect capabilities.",
      ],
      [
        "Select a host",
        [
          ["outpost host use dev", "Set the active host"],
          ["outpost --host work status", "Target a host on a single command"],
        ],
        "Set the active host, or target a host on an individual command with --host NAME.",
      ],
      [
        "Manage AWS lifecycle",
        [
          ["outpost host start dev", "Start a stopped EC2 instance"],
          ["outpost host stop dev", "Stop the instance to save cost"],
          ["outpost host restart dev", "Restart the instance"],
          [
            "outpost host resize dev --instance-type t3.large",
            "Change the instance type",
          ],
        ],
        "Start, stop, restart, or resize a cloud-managed host.",
      ],
      [
        "Remove or destroy",
        [
          ["outpost host remove dev", "Remove host from local config only"],
          ["outpost host destroy dev", "Terminate the EC2 instance"],
        ],
        "Remove local configuration, or terminate the AWS instance. These are different operations.",
      ],
    ],
  ],
  [
    "projects",
    "Projects & Compose",
    "Keep your repository local while Compose runs on the remote host.",
    [
      [
        "Initialize",
        [
          ["outpost init", "Detect Compose files and write project config"],
          [
            "outpost init --name my-api --write-gitignore",
            "Initialize with a project name and gitignore",
          ],
          ["outpost init --no-compose", "Initialize a script-only repository"],
        ],
        "Detect Compose files and write .outpost/project.yaml, or initialize a script-only repository.",
      ],
      [
        "Manage services",
        [
          ["outpost compose up -d", "Start services in the background"],
          ["outpost compose ps", "List running services"],
          ["outpost compose logs -f", "Follow service logs"],
          ["outpost compose exec api sh", "Open a shell in a service"],
          ["outpost compose down", "Stop and remove the stack"],
        ],
        "Start, inspect, debug, enter, and stop your stack.",
      ],
      [
        "Use Docker",
        [
          ["outpost docker ps", "List containers on the remote engine"],
          ["outpost docker logs my-container", "Read container logs"],
        ],
        "Run Docker commands against the remote engine.",
      ],
      [
        "Move volumes",
        [
          ["outpost compose volumes export", "Archive named volumes locally"],
          ["outpost compose volumes import", "Restore volumes on this host"],
          ["outpost compose volumes list", "List archived volumes"],
        ],
        "Archive named volumes locally and restore them on another host.",
      ],
    ],
  ],
  [
    "mirror",
    "Remote mirror",
    "Sync a repository and execute work remotely without moving generated output back to your laptop.",
    [
      [
        "Sync and run",
        [
          ["outpost mirror sync", "Mirror the repository to the host"],
          [
            "outpost mirror run node scripts/generate.js",
            "Run a command in the remote project directory",
          ],
        ],
        "Mirror the repository and run a command in its remote directory.",
      ],
      [
        "Keep a session alive",
        [
          [
            "outpost mirror run -d --name gen node scripts/generate-40k.js",
            "Start detached work in tmux",
          ],
          ["outpost mirror sessions list", "List detached sessions"],
          ["outpost mirror sessions status gen", "Check session status"],
          ["outpost mirror sessions attach gen", "Reconnect to a session"],
          ["outpost mirror sessions kill gen", "Stop a detached session"],
        ],
        "Run detached work in tmux and reconnect later.",
      ],
      [
        "Use Python remotely",
        [
          [
            "outpost mirror setup-python",
            "Create a remote virtual environment",
          ],
          [
            "outpost mirror run python scripts/train.py",
            "Run Python in the remote environment",
          ],
          ["outpost mirror shell", "Open a shell in the mirrored project"],
        ],
        "Create a remote-only virtual environment and work inside it.",
      ],
    ],
  ],
  [
    "connections",
    "Connections",
    "Bring remote services back to familiar localhost addresses.",
    [
      [
        "Forward services",
        [
          ["outpost connect", "Forward all published ports"],
          ["outpost connect --service api", "Forward one service"],
          ["outpost connect --port 9090:80", "Forward a custom port mapping"],
        ],
        "Forward all published ports, one service, or a custom mapping.",
      ],
      [
        "Inspect or stop",
        [
          ["outpost connect --status", "View active forwarding sessions"],
          ["outpost connect --down", "Stop port forwarding"],
        ],
        "View active sessions or stop forwarding.",
      ],
      [
        "Connect a machine",
        [
          [
            "outpost machine connect ubuntu-dev --port 8080:80",
            "Forward a port from an Incus machine",
          ],
        ],
        "Forward a port from an Incus machine.",
      ],
    ],
  ],
  [
    "sharing",
    "Team sharing",
    "Share runtime access while the owner keeps host, invitation, and cloud control.",
    [
      [
        "Create and manage invitations",
        [
          ["outpost invite create", "Create a new invitation code"],
          ["outpost invite list", "List pending invitations"],
          ["outpost invite approve DEVICE_ID", "Approve a teammate device"],
          ["outpost invite revoke DEVICE_ID", "Revoke access for a device"],
        ],
        "Create invitation codes, approve devices, or revoke access.",
      ],
      [
        "Join a host",
        [
          [
            "outpost invite join CODE --hostname 203.0.113.10 --user ubuntu --label my-laptop",
            "Join with a code and wait for approval",
          ],
        ],
        "Join with a code and wait for owner approval.",
      ],
    ],
  ],
  [
    "clusters",
    "Kubernetes",
    "Create and use kind clusters on the remote host without a local kubectl runtime.",
    [
      [
        "Create and inspect",
        [
          ["outpost cluster create dev", "Create a named kind cluster"],
          [
            "outpost cluster create staging --workers 2",
            "Create a cluster with worker nodes",
          ],
          ["outpost cluster list", "List clusters on the host"],
          ["outpost cluster status dev", "Inspect cluster state"],
        ],
        "Create named clusters and inspect their state.",
      ],
      [
        "Run kubectl",
        [
          [
            "outpost kubectl --cluster dev get nodes",
            "Run kubectl against a named cluster",
          ],
          [
            "outpost kubectl --cluster dev apply -f ./manifest.yaml",
            "Apply manifests remotely",
          ],
        ],
        "Execute kubectl remotely against a named cluster.",
      ],
      [
        "Delete",
        [["outpost cluster delete dev", "Delete a cluster when finished"]],
        "Delete a cluster when it is no longer needed.",
      ],
    ],
  ],
  [
    "machines",
    "Linux machines",
    "Use lightweight system containers by default, or full VMs when the host has KVM.",
    [
      [
        "Create a container",
        [
          [
            "outpost machine create ubuntu-dev --image ubuntu:24.04",
            "Create a lightweight Linux container",
          ],
          [
            "outpost machine create big-dev --image ubuntu:24.04 --cpu 2 --memory 2GiB --disk 20GiB",
            "Create a larger machine with custom resources",
          ],
        ],
        "Create a right-sized Linux machine on the remote host.",
      ],
      [
        "Work with a machine",
        [
          ["outpost machine shell ubuntu-dev", "Open an interactive shell"],
          [
            "outpost machine exec ubuntu-dev -- uname -a",
            "Run a command in the machine",
          ],
          [
            "outpost machine copy ./app ubuntu-dev:/tmp/app",
            "Copy files into the machine",
          ],
          [
            "outpost machine copy ubuntu-dev:/tmp/output.log ./output.log",
            "Copy files back to your laptop",
          ],
        ],
        "Open a shell, run a command, or copy files.",
      ],
      [
        "Lifecycle",
        [
          ["outpost machine stop ubuntu-dev", "Stop a running machine"],
          [
            "outpost machine snapshot create ubuntu-dev",
            "Capture a machine snapshot",
          ],
          ["outpost machine delete ubuntu-dev", "Delete the machine"],
        ],
        "Stop, snapshot, or delete a machine.",
      ],
      [
        "Create a VM",
        [
          ["outpost host capabilities", "Check whether the host supports KVM"],
          [
            "outpost machine create vm-dev --image ubuntu:24.04 --virtual-machine --cpu 2 --memory 2GiB --disk 20GiB",
            "Create a full VM when KVM is available",
          ],
        ],
        "VMs require KVM and more host resources.",
      ],
    ],
  ],
  [
    "monitoring",
    "Monitoring & cleanup",
    "Inspect health, capacity, disk pressure, and reclaimable resources.",
    [
      [
        "Inspect",
        [
          ["outpost status", "Review overall workload health"],
          ["outpost top", "Show live resource usage"],
          ["outpost top --watch", "Watch usage continuously"],
          ["outpost capacity", "Inspect host capacity"],
          ["outpost disk", "Check disk pressure"],
        ],
        "Review workload health, live resource usage, capacity, and disk.",
      ],
      [
        "Clean up",
        [
          ["outpost prune --dry-run", "Preview reclaimable resources"],
          ["outpost prune", "Remove unused containers and images"],
          ["outpost prune volumes", "Remove unused volumes"],
        ],
        "Preview or remove stopped containers, unused images, build cache, and volumes.",
      ],
    ],
  ],
  [
    "reference",
    "Command reference",
    "Every command, including passthrough tools and local configuration.",
    [
      [
        "Remote runtime commands",
        [
          ["outpost docker [args...]", "Run any Docker command remotely"],
          [
            "outpost compose build|pull|up|down|ps|logs|exec",
            "Run Compose commands remotely",
          ],
          [
            "outpost kubectl --cluster NAME [args...]",
            "Run any kubectl command remotely",
          ],
          ["outpost connect [flags]", "Forward Compose ports to localhost"],
        ],
        "Docker, Compose, and kubectl pass their remaining arguments to the remote tool, so the full underlying command surface remains available.",
      ],
      [
        "Host and provider commands",
        [
          [
            "outpost provider login aws [flags]",
            "Validate and store AWS credentials",
          ],
          ["outpost host add|create NAME", "Register or provision a host"],
          [
            "outpost host list|use|verify|capabilities",
            "Inspect and select hosts",
          ],
          [
            "outpost host start|stop|restart|resize NAME",
            "Manage a cloud host",
          ],
          [
            "outpost host remove|destroy NAME",
            "Forget locally or terminate remotely",
          ],
        ],
        "Use remove to delete local registration only; use destroy to terminate a cloud host. Host creation also supports --provider, --region, --profile, --instance-type, --ssh-cidr, and --no-cleanup.",
      ],
      [
        "Projects, mirrors, and sessions",
        [
          ["outpost init [flags]", "Create .outpost/project.yaml"],
          ["outpost mirror sync|shell", "Sync or open the remote project"],
          [
            "outpost mirror setup-python [flags]",
            "Create a remote Python environment",
          ],
          [
            "outpost mirror run [flags] -- COMMAND",
            "Run a remote project command",
          ],
          [
            "outpost mirror sessions list|status|attach|kill",
            "Manage detached sessions",
          ],
        ],
        "Use --no-sync and --no-venv with mirror run when you need to control synchronization and virtual-environment activation.",
      ],
      [
        "Clusters and machines",
        [
          ["outpost cluster create|list|status|delete", "Manage kind clusters"],
          [
            "outpost machine create|list|status",
            "Create and inspect Incus machines",
          ],
          [
            "outpost machine start|stop|restart NAME",
            "Manage machine lifecycle",
          ],
          [
            "outpost machine shell|exec|copy|connect",
            "Work inside or connect to a machine",
          ],
          [
            "outpost machine snapshot create|list|delete",
            "Manage machine snapshots",
          ],
          ["outpost machine delete NAME", "Delete a machine"],
        ],
        "Machine create accepts --image, --cpu, --memory, --disk, and --virtual-machine. Copy accepts --recursive/-r; connect accepts --port and --bind.",
      ],
      [
        "Sharing, cleanup, and local tools",
        [
          [
            "outpost invite create|join|list|approve|revoke",
            "Manage team access",
          ],
          ["outpost capacity|status|top|disk", "Inspect health and resources"],
          [
            "outpost prune [volumes|clusters|machines]",
            "Preview or reclaim resources",
          ],
          ["outpost reset", "Clear local Outpost configuration"],
          ["outpost completion SHELL", "Generate shell completion"],
          ["outpost help [COMMAND]", "Open built-in command help"],
        ],
        "Run prune with --dry-run first. The explicit target forms support --force; destructive actions should be reviewed before using --yes.",
      ],
    ],
  ],
  [
    "options",
    "Using any command",
    "Common flags belong with the command they modify and work across the command surface.",
    [
      [
        "Target and inspect",
        [
          [
            "outpost --host NAME status",
            "Target a host without changing the active host",
          ],
          ["outpost --json status", "Return machine-readable JSON"],
          ["outpost --debug host verify", "Show verbose diagnostics"],
          ["outpost --help", "Show help for the current command"],
        ],
        "Place --host, --json, --debug, --yes, or --help with the command you are running. They are not a separate command family.",
      ],
      [
        "Confirm safely",
        [
          ["outpost prune --dry-run", "Preview cleanup candidates"],
          [
            "outpost prune volumes --force --yes",
            "Confirm an explicit volume cleanup",
          ],
          ["outpost host destroy NAME", "Review the termination prompt"],
        ],
        "Use --yes for trusted automation or after reviewing the consequence. Shared-host operations warn when other members may be affected.",
      ],
    ],
  ],
];

function commandCopyText(lines) {
  return lines.map(([command]) => command).join("\n");
}
function CommandSnippet({ lines, title }) {
  let copyText = commandCopyText(lines);

  return (
    <div className="command-code">
      <pre>
        {lines.map(([command, note], index) => (
          <React.Fragment key={`${command}-${index}`}>
            {command}
            {note ? <span className="command-note"> # {note}</span> : null}
            {index < lines.length - 1 ? "\n" : ""}
          </React.Fragment>
        ))}
      </pre>
      <button
        aria-label={`Copy ${title} command`}
        onClick={() => navigator.clipboard?.writeText(copyText)}
      >
        Copy
      </button>
    </div>
  );
}
function Logo() {
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
function RotatingText({ items, interval = 2600 }) {
  let reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let [index, setIndex] = useState(0);
  let [visible, setVisible] = useState(true);

  useEffect(() => {
    if (reducedMotion || items.length < 2) {
      return;
    }

    let timer = window.setInterval(() => {
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
function CopyCommand({ command, display = command, className = "" }) {
  let [copied, setCopied] = useState(false);

  let copy = () => {
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
        aria-label={`Copy install command: ${command}`}
      >
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
  );
}
function GitHubStar() {
  let [stars, setStars] = useState(null);

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
function Header({ usage }) {
  return (
    <header>
      <nav className="wrap">
        <Logo />
        <div className="nav-links">
          <a className={usage ? "active" : ""} href="#usage">
            Usage guide
          </a>
          <GitHubStar />
        </div>
      </nav>
    </header>
  );
}
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
function TerminalDemo({ script, loopDelay = 2400, whenVisible = false }) {
  let reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let [history, setHistory] = useState([]);
  let [typing, setTyping] = useState("");
  let [inView, setInView] = useState(!whenVisible);
  let timers = useRef([]);
  let rootRef = useRef(null);
  let bodyRef = useRef(null);

  let clearTimers = useCallback(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
  }, []);

  let schedule = useCallback((fn, ms) => {
    let id = setTimeout(fn, ms);
    timers.current.push(id);
    return id;
  }, []);

  let run = useCallback(() => {
    clearTimers();
    setHistory([]);
    setTyping("");

    let delay = 0;
    let typeSpeed = 42;
    let linePause = 320;
    let blockPause = 700;

    script.forEach((block, blockIndex) => {
      for (let i = 0; i < block.command.length; i++) {
        let text = block.command.slice(0, i + 1);
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

    let node = rootRef.current;
    if (!node) {
      return;
    }

    let observer = new IntersectionObserver(
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
    let body = bodyRef.current;
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
function Topology() {
  return (
    <div className="topology">
      <svg
        viewBox="0 0 520 370"
        role="img"
        aria-label="Local Outpost CLI connected to a remote Linux host"
      >
        <defs>
          <pattern
            id="dots"
            width="26"
            height="26"
            patternUnits="userSpaceOnUse"
          >
            <circle cx="2" cy="2" r="1" fill="#60a5fa" opacity=".22" />
          </pattern>
          <filter id="glow">
            <feGaussianBlur stdDeviation="5" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <rect width="520" height="370" fill="url(#dots)" />
        <path
          d="M125 185 C205 110 292 110 360 185 S400 260 430 225"
          fill="none"
          stroke="#2563ff"
          strokeWidth="1.5"
          opacity=".75"
        />
        <path
          d="M125 185 H360"
          stroke="#60a5fa"
          strokeDasharray="4 7"
          opacity=".45"
        />
        {[
          [125, 185],
          [360, 185],
          [430, 225],
        ].map(([cx, cy]) => (
          <React.Fragment key={cx}>
            <circle cx={cx} cy={cy} r="8" fill="#2563ff" filter="url(#glow)" />
            <circle cx={cx} cy={cy} r="18" fill="none" stroke="#2563ff" />
          </React.Fragment>
        ))}
        <text x="82" y="238">
          LOCAL
        </text>
        <text x="82" y="258" className="bright">
          outpost CLI
        </text>
        <text x="315" y="145">
          SSH →
        </text>
        <text x="325" y="238">
          REMOTE
        </text>
        <text x="325" y="258" className="bright">
          Linux host
        </text>
        <text x="390" y="285" className="blue">
          services
        </text>
      </svg>
    </div>
  );
}
function Capabilities() {
  let features = [
    [
      ">_",
      "Remote Compose",
      "Sync your project and run Docker Compose stacks on a shared Linux host.",
    ],
    [
      "◈",
      "Kubernetes with kind",
      "Create named clusters and run kubectl remotely. No local runtime stack required.",
    ],
    [
      "□",
      "Linux machines",
      "Launch lightweight Incus containers, or full VMs when the host supports KVM.",
    ],
    [
      "↔",
      "Local port forwarding",
      "Reach remote services at localhost while the workload stays on the host.",
    ],
    [
      "⟳",
      "Remote mirror",
      "Sync your repo, run commands remotely, and keep detached sessions alive.",
    ],
    [
      "♧",
      "Team sharing",
      "Invite collaborators with approval. Keep ownership, credentials, and boundaries clear.",
    ],
  ];

  return (
    <section id="capabilities" className="light-section">
      <div className="wrap">
        <div className="section-head">
          <div>
            <div className="eyebrow">One command surface</div>
            <h2>
              Everything useful on the host. Nothing heavy on your laptop.
            </h2>
          </div>
          <p className="section-copy">
            Run the workloads you need on infrastructure you can inspect, share,
            and control.
          </p>
        </div>
        <div className="features">
          {features.map(([icon, title, copy]) => (
            <article className="feature" key={title}>
              <div className="icon">{icon}</div>
              <h3>{title}</h3>
              <p>{copy}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
function Home() {
  return (
    <>
      <section className="hero wrap grid-bg">
        <div>
          <h1>
            Your remote dev environment. <span>Anywhere.</span>
          </h1>
          <p className="lead">
            Outpost turns a remote Linux host into a shared development
            environment you control from your local terminal.
            <span className="stacked-line">
              Keep your local workflow light and move the runtime to a remote
              machine.
            </span>
          </p>
          <div className="actions">
            <CopyCommand
              command={INSTALL_COMMAND}
              display={INSTALL_COMMAND_DISPLAY}
            />
          </div>
        </div>
        <TerminalDemo script={HERO_TERMINAL_DEMO} />
      </section>
      <Capabilities />
      <section id="how-it-works">
        <div className="wrap architecture">
          <div>
            <div className="eyebrow">The connection</div>
            <h2>
              Local workflow.
              <span className="stacked-line">Remote runtime.</span>
            </h2>
            <p className="arch-copy">
              Outpost connects over SSH, bootstraps missing tools on first use,
              mirrors project files, and brings services back to your localhost.
              There is no permanent Outpost agent on the server.
            </p>
            <div className="arch-list">
              <div>
                <strong className="mono">Your terminal</strong>
                <span>Commands, projects, credentials</span>
              </div>
              <div>
                <strong className="mono">SSH connection</strong>
                <span>Controlled path to the host</span>
              </div>
              <div>
                <strong className="mono">Remote host</strong>
                <span>Docker, kind, Incus, services</span>
              </div>
            </div>
          </div>
          <Topology />
        </div>
      </section>
      <section className="workflow light-section">
        <div className="wrap">
          <div className="section-head">
            <div>
              <div className="eyebrow">Start from where you are</div>
              <h2>
                Bring your VPS. Or provision one on{" "}
                <RotatingText items={["AWS", "Digital Ocean", "GCP"]} />.
              </h2>
            </div>
            <p className="section-copy">
              A short path from a repository to a running stack on
              infrastructure you own.
            </p>
          </div>
          <TerminalDemo script={WORKFLOW_TERMINAL_DEMO} />
        </div>
      </section>
      <section className="cta">
        <div className="wrap cta-inner">
          <div>
            <div className="eyebrow">&gt;_ Keep your laptop light.</div>
            <h2>
              Develop remotely.
              <span className="stacked-line">Work from anywhere.</span>
            </h2>
          </div>
          <CopyCommand
            command={INSTALL_COMMAND}
            display={INSTALL_COMMAND_DISPLAY}
          />
        </div>
      </section>
    </>
  );
}
function Usage() {
  let initial =
    groups.find((g) => `#${g[0]}` === window.location.hash)?.[0] ||
    groups[0][0];
  let [selected, setSelected] = useState(initial);
  let group = groups.find((g) => g[0] === selected) || groups[0];
  return (
    <main className="usage-page" id="usage">
      <section className="usage-hero">
        <div className="wrap">
          <div className="eyebrow">Usage reference</div>
          <h1>
            Run it there. <span>Control it here.</span>
          </h1>
          <p className="lead">
            The complete command surface for hosts, projects, clusters,
            machines, connections, and teams.
          </p>
        </div>
      </section>
      <section className="usage-body">
        <div className="wrap usage-layout">
          <aside className="usage-nav">
            <div className="mono nav-label">COMMANDS</div>
            {groups.map((g) => (
              <a
                key={g[0]}
                className={g[0] === selected ? "selected" : ""}
                href={`#${g[0]}`}
                onClick={(e) => {
                  e.preventDefault();
                  setSelected(g[0]);
                  window.history.replaceState(null, "", `#${g[0]}`);
                }}
              >
                {g[1]}
                <span>→</span>
              </a>
            ))}
          </aside>
          <div className="command-content">
            <div className="eyebrow">{group[1]}</div>
            <h2>{group[2]}</h2>
            {group[3].map(([title, lines, description]) => (
              <article className="command-card" key={title}>
                <div className="command-heading">
                  <h3>{title}</h3>
                  <span className="mono">outpost</span>
                </div>
                <p>{description}</p>
                <CommandSnippet lines={lines} title={title} />
              </article>
            ))}
          </div>
        </div>
      </section>
    </main>
  );
}
function Footer() {
  return (
    <footer>
      <div className="wrap">
        <span>© 2026 Outpost · Remote power. Local control.</span>
        <span>
          <a href={GITHUB}>GitHub</a> · <a href="#usage">Usage</a>
        </span>
      </div>
    </footer>
  );
}
function App() {
  let [usage, setUsage] = useState(
    window.location.hash === "#usage" ||
      groups.some((g) => `#${g[0]}` === window.location.hash),
  );
  useEffect(() => {
    let f = () =>
      setUsage(
        window.location.hash === "#usage" ||
          groups.some((g) => `#${g[0]}` === window.location.hash),
      );
    window.addEventListener("hashchange", f);
    return () => window.removeEventListener("hashchange", f);
  }, []);
  return (
    <>
      <Header usage={usage} />
      {usage ? <Usage /> : <Home />}
      <Footer />
    </>
  );
}
createRoot(document.getElementById("root")).render(<App />);
