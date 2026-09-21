# AGENTS.md

Guidance for AI coding agents working in this repository. `CLAUDE.md` is a symlink to this file.

---

## Licensing

- Source files should carry an SPDX identifier where applicable.

---

## Directory structure

| Path | Purpose |
| --- | --- |
| `docs/` | Any documentation. |
| `docs/agents/` | Documentation and similar written output produced by AI agents and assistants — notes, memory, plans, research reports, analyses. |
| `hack/` | Developer tooling: scripts, generators, and anything else that is not compiled into the application itself. |

This list is not exhaustive and will grow as the template does.

---

## Container builds

The image is defined in `Containerfile`. `Dockerfile` and `.dockerignore` are symlinks to `Containerfile` and `.containerignore` — edit the originals, never the symlinks, and never replace a symlink with a regular file.

Build and run:

```sh
podman build -t robbe:dev .
podman build --build-arg VERSION=v1.2.3 --build-arg REVISION="$(git rev-parse HEAD)" -t robbe:v1.2.3 .
podman run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges robbe:dev
```

Build arguments: `VERSION` and `REVISION` are stamped into the binary via `-ldflags -X` and into the OCI labels. `GOARCH` defaults to the build platform's architecture; cross-build with `--build-arg GOARCH=arm64`. `GOOS` is fixed to `linux` because the runtime stage is a Linux image.

Rules for agents changing this file:

- Do not add a package manager, shell, or debugging tools to the runtime stage. The runtime base is `gcr.io/distroless/static-debian12:nonroot` precisely because it contains none, and a shell in the final image is a security regression.
- Do not remove or weaken `USER 65532:65532`, the `--chown=root:root --chmod=0555` on the binary, `CGO_ENABLED=0`, or the `-trimpath`/`-buildvcs=false`/`-buildid=` build flags. Each is load-bearing and the reason is in a comment beside it.
- Keep image references fully qualified (`docker.io/library/...`). Podman does not assume Docker Hub for unqualified names.
- Anything that must not reach an image layer belongs in `.containerignore`, not in a `COPY` exclusion.

---

## Documentation scope

By default, AI assistants may only write documentation-style content inside the `docs/agents/` directory. This covers documentation, notes, memory files, plans, designs, research reports, analyses, and anything similar.

Documentation anywhere else in the repository — including `README.md`, `docs/` outside `docs/agents/`, `CONTRIBUTING.md`, this file, package-level doc comments written as standalone documentation work, and any other prose intended for human readers — is off-limits unless a human user explicitly instructs the agent to change that specific file or location. A general request such as "improve the docs" is not sufficient authorization for files outside `docs/agents/`; ask which files to touch.

This rule governs documentation only. It does not restrict ordinary source-code changes, which follow the usual review process, and it does not apply to code comments written as part of a code change the user asked for.

---

## Attribution

### For AI-assisted commits

AI assistants MUST NOT add `Signed-off-by` tags — only a human can certify the DCO. The human committer is responsible for:

- Reviewing all AI-generated code.
- Ensuring licensing compliance.
- Adding their own `Signed-off-by`.
- Taking full responsibility for the contribution.

AI assistants MUST NOT add `Co-authored-by` tags.

When AI assistance materially shaped a commit, add an attribution trailer:

```
Assisted-by: AGENT_NAME:MODEL_VERSION [TOOL1] [TOOL2]
```

Examples:

```
Assisted-by: Cursor:claude-sonnet-4.5
Assisted-by: Copilot:gpt-5
Assisted-by: Claude:claude-opus-4
```

Do not list basic tools (git, go, make, editors).

Follow the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) standard for commit messages and PR titles. Restrict to the core types — `feat`, `fix`, `chore` — plus a `!` after the type/scope or a `BREAKING CHANGE:` footer to flag breaking changes. Do not use other types (`docs`, `refactor`, `style`, `perf`, `test`, `build`, `ci`, etc.).

### Pull-request titles and semantic-release

This project uses **semantic-release** to automate versioning. semantic-release reads the **squash-merge commit message**, which GitHub sets to the **PR title**. A PR whose title does not follow Conventional Commits will **not** trigger a release.

**Format:** `type(scope): description`

- Allowed types: `feat`, `fix`, `chore` — no others (`docs`, `refactor`, `style`, `perf`, `test`, `build`, `ci` are not permitted).
- Scope is optional but recommended (e.g. `feat(template): ...`, `fix(api): ...`).
- For breaking changes, append `!` after the type/scope (`feat!: ...` or `feat(api)!: ...`) or add a `BREAKING CHANGE:` footer in the PR body.
- Description starts lowercase, no trailing period.

**Good:**

- `feat(template): add support for custom metadata fields`
- `fix: correct template variable substitution`
- `chore!: drop support for legacy template format`

**Bad:**

- `Add support for custom metadata fields` (no type prefix — semantic-release ignores this)
- `feat: Add Support For Custom Metadata Fields.` (capitalized, trailing period)
- `docs: update README` (type `docs` not allowed)

When creating PRs via `gh pr create`, always pass `--title` in this format. Reference: <https://www.conventionalcommits.org/en/v1.0.0/>

### For AI-assisted PR comments

PR comments are for humans only. AI assistants MUST NOT write or post PR comments (review comments, issue comments, or approvals) under any circumstances — not even when explicitly asked to "post" or "submit" one, and not via `gh`, the GitHub API, or any other tool.

If a user wants help drafting a PR comment, the agent should:

- Summarize its own output (e.g. a code review or analysis).
- Let the user review that summary.
- Draft a concise, human-audience comment as text in the conversation, for the user to copy, edit as needed, and post **themselves**.
- Include an `Assisted-by` trailer (same format as commits, see above) at the end of the draft, so the human-posted comment discloses AI assistance.

This restriction applies to PR **comments** only. AI-assisted PR **descriptions** are allowed — an agent may write and post/update the PR description body itself (e.g. via `gh pr create`/`gh pr edit`).

The user is always the one who writes and posts the comment. The agent's role ends at producing a draft.
