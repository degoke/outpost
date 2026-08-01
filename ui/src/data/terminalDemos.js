export const HERO_TERMINAL_DEMO = [
  {
    command: "outpost init",
    lines: [
      { className: "ok", prefix: "✓", text: 'Initialized project "my-api"' },
      {
        className: "route",
        prefix: "→",
        text: "Opening remote shell — type exit to return",
      },
      { className: "route", prefix: "→", text: "Syncing project to remote…" },
      { className: "ok", prefix: "✓", text: "Project shell ready" },
    ],
  },
  {
    command: "outpost run -- npm test",
    lines: [
      {
        className: "ok",
        prefix: "✓",
        text: "Running inside outpost-dev-my-api",
      },
      { className: "ok", prefix: "✓", text: "Tests passed" },
    ],
  },
];

export const WORKFLOW_TERMINAL_DEMO = [
  {
    command:
      "outpost provider login aws --profile my-profile --region eu-west-1",
    lines: [
      {
        className: "ok",
        prefix: "✓",
        text: "AWS login OK (account 123456789012, region eu-west-1)",
      },
    ],
  },
  {
    command: "outpost host create dev --provider aws --region eu-west-1",
    lines: [
      { className: "route", prefix: "→", text: "Bootstrapping remote host…" },
      {
        className: "ok",
        prefix: "✓",
        text: 'Host "dev" created and ready',
      },
    ],
  },
  {
    command: "outpost init --name my-api",
    lines: [
      { className: "ok", prefix: "✓", text: 'Initialized project "my-api"' },
    ],
  },
  {
    command: "outpost compose up",
    lines: [{ className: "ok", prefix: "✓", text: "Stack my-api is running" }],
  },
  {
    command: "outpost open",
    lines: [{ type: "forward", from: "http://127.0.0.1:8080", to: "api:8080" }],
  },
];

export const TICKER_COMMANDS = [
  "outpost init",
  "outpost shell",
  "outpost run -- npm test",
  "outpost migrate --from old --to new",
  "outpost compose up",
  "outpost open",
  "outpost cluster up",
  "outpost machine shell",
  "outpost host create dev",
];

export const BENTO_FEATURES = [
  {
    id: "env",
    featured: true,
    icon: "⟳",
    title: "Managed environments",
    copy: "One container per project. Devcontainer support, rsync sync, Starship shell, and background watch while you work.",
  },
  {
    icon: ">_",
    title: "Remote Compose",
    copy: "Sync your project and run Docker Compose stacks on a shared Linux host.",
  },
  {
    icon: "◈",
    title: "Kubernetes",
    copy: "Project-scoped kind or k3d clusters with a local kubeconfig at .outpost/kubeconfig.",
  },
  {
    icon: "□",
    title: "Linux machines",
    copy: "Incus containers by default; full VMs when the host supports KVM.",
  },
  {
    icon: "↔",
    title: "Port forwarding",
    copy: "Reach remote services at localhost while workloads stay on the host.",
  },
  {
    icon: "⇄",
    title: "Host migration",
    copy: "Move containers, volumes, Kubernetes state, and metadata with outpost migrate.",
  },
  {
    icon: "♧",
    title: "Team sharing",
    copy: "Invite collaborators with approval. Owners keep credentials and cloud control.",
  },
];
