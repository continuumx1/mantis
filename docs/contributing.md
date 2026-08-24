# Contributing

Mantis is being built in the open. **You don't need to be a developer to
contribute** — reporting bugs, suggesting features, testing releases,
writing docs, or just opening an issue all count. Even feedback is a
contribution.

## Where to go

- **[Discord](https://discord.gg/ZTB4eGfCxa)** — the fastest way to reach
  the maintainers. Ask questions, propose ideas, talk through architecture,
  or report what's confusing before you build it.
- **[GitHub](https://github.com/continuumx1/mantis)** — source code,
  issues, and pull requests. Star it, watch releases, or read the code
  directly.

## Areas we're looking for help in

| Area | What that looks like |
|---|---|
| Kubernetes & Platform Engineering | Relationship logic, RBAC, cluster compatibility |
| DevOps / SRE | Helm chart, deployment patterns, real-world cluster testing |
| Frontend & UI/UX | The graph UI itself — interaction, clarity, accessibility |
| Backend (Go) | The relationship engine and API — `internal/graph`, `internal/engine` |
| Documentation & technical writing | Guides, examples, clearer explanations — including this site |
| Testing & security | Edge cases, RBAC scenarios, responsible vulnerability reports |

## Making a change

Keep pull requests small and focused, follow [Conventional Commits](https://www.conventionalcommits.org/),
and include tests for new relationship logic — `internal/graph`'s existing
tests use a fake Kubernetes client, so no real cluster is needed to
contribute. `go build ./... && go vet ./... && go test ./...` should pass
clean before you open a PR.

Formal `CONTRIBUTING.md` / `CODE_OF_CONDUCT.md` guidelines are on the way;
until then, the [Discord](https://discord.gg/ZTB4eGfCxa) is the place to
ask "how would you want this done?" before investing a lot of time in a
change.
