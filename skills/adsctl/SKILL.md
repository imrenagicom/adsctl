---
name: adsctl
description: "Use the adsctl CLI to inspect Meta ad accounts, query performance and insights, monitor delivery, manage campaigns/ad sets/ads and budgets, upload creative assets, and create or recover campaign launches. Use when the user requests adsctl operations or agent workflows built on adsctl. Meta is currently supported; Google commands are not implemented."
---

# adsctl

Operate the installed CLI using its compiled contract. This skill targets `adsctl meta`; it does not depend on an Ads Manager connector.

## Establish executable and scope

1. Find `adsctl` on PATH. In an adsctl checkout, `bin/adsctl` is also usable; if absent, `make build` builds it. Do not assume the skill's installation directory contains the Go repository.
2. Identify the requested credential profile, ad account, resource IDs, and reporting period. Prefer explicit `--profile` and `--account` even for reads. If scope is missing, inspect configured profile names and accessible accounts; ask when several could satisfy the request. Never silently switch clients after an access failure.
3. Discover the installed command and supported flags before unfamiliar operations:

```sh
adsctl commands --platform meta --capability read
adsctl commands --platform meta --capability write
adsctl meta schema get campaigns --operation create
adsctl meta campaigns create --help
```

Use `--capability report-job` for async Insights and `--capability local` for local operations. Compiled help and schemas take precedence over examples or design documents. Keep account/resource IDs as strings; use an account ID such as `act_123`.

### Credentials when setup is requested

```sh
adsctl config profiles list
adsctl config profiles set client-read --platform meta --token-env META_CLIENT_READ_TOKEN --mode read
adsctl config validate
adsctl meta auth check --profile client-read --account act_123
adsctl meta accounts list --profile client-read --all
```

The runner must supply the referenced environment variable or raw-token file. Configure a write profile with `--mode write` and a user-selected `--token-env` or `--token-file` only when needed. Never put tokens in command arguments, chat, payload files, or committed configuration. Do not print secret environment values or token files. The CLI does not perform OAuth or token refresh. `auth check` establishes account read access, not every future write permission. Profile API versions are configurable; inspect the configured version rather than assuming live compatibility.

## Choose the workflow

- Performance queries, comparisons, exports, delivery diagnosis, monitoring, or snapshots: read [reporting.md](references/reporting.md). For a full funnel audit, benchmark evaluation, and media-buyer recommendations on running campaigns, use [`adsctl-meta-analyst`](../adsctl-meta-analyst/SKILL.md). To explain why performance changed between two periods, use its [change investigation](../adsctl-meta-analyst/references/change-investigation.md).
- Resource changes, budgets, media, campaign creation, activation, or interrupted launches: read [management.md](references/management.md).

Examples use `$PROFILE`, `$WRITE_PROFILE`, `$ACCOUNT`, and `$STATE_DIR` as placeholders. Resolve these to the intended scope; keep the same private state directory for every operation on a run. A write profile requires explicit `--profile` and `--account` flags. Read profiles can create async report jobs; that effect does not change advertising.

## Interpret every result

Default JSON has `schema_version`, `ok`, `data`, `meta`, `warnings`, and `error`. Preserve stdout and stderr separately and inspect both on failure: partial operations can emit an `ok:false` receipt on stdout and an error on stderr. Check the process exit, envelope `ok`, and `meta.complete` together. A successful one-page list can still be incomplete. JSONL uses meta/data/error/end records; require its end record and inspect success/completeness. Tables are lossy; use JSON for agent reasoning.

- Lists default to one page. Use `--all` for complete analysis, but respect `--max-items` and timeouts. Do not present truncated results as a global ranking or complete inventory. Keep continuation cursors with their original profile, account, version, and query.
- Missing or unavailable metrics are not zero. Keep currencies separate. Distinguish configured status, effective status, and observed delivery.
- Exit 9 means incomplete evidence or an applied-but-unverified change; inspect the receipt. Exit 10 means an unknown mutation outcome; reconcile before retrying. Exit 11 with `monitor check --fail-on-finding` can mean a completed evaluation found an issue, not a transport failure.
- GET retries are already bounded by the CLI. Do not wrap commands in blanket retries: POST/DELETE ambiguity can duplicate changes. Fix credential, permission, scope, or validation errors in the selected scope.

For writes, use the user's existing authorization and preserve its limits. An insight request or recommendation does not authorize changing spend. Prepare the concrete payload/plan and dry-run before seeking any missing authorization. Paused creation and activation are separate actions; do not infer activation from a request to create paused objects.

## Report the outcome

State the profile/account, dates and attribution for analysis, important findings, and evidence limitations. For changes, include affected IDs, verified versus uncertain outcomes, and the run ID/state directory when applicable. Do not claim delivery from `ACTIVE` status or acceptance from local validation. Live Meta compatibility must be established from actual account evidence; repository fixture tests alone do not establish it.
