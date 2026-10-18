# adsctl

A Go and Cobra CLI for inspecting, monitoring, creating and managing advertising. Meta is implemented first; Google is a future adapter. The CLI is designed for AI agents and shell automation: JSON by default, explicit account scope, external credentials, bounded reads and recoverable launch runs.

The repository includes three shared agent skills in `skills/`:
- [`adsctl`](skills/adsctl/SKILL.md) for reporting, monitoring, management, and launch recovery workflows;
- [`adsctl-meta-analyst`](skills/adsctl-meta-analyst/SKILL.md) for structured Meta campaign performance audits, funnel diagnostics, and media-buyer recommendations;
- [`adsctl-setup`](skills/adsctl-setup/SKILL.md) for CLI installation, developer apps, user and system-user tokens, asset permissions, secret references, and bounded authentication checks.

All three skills are included in release archives.

## Install

On macOS or Linux, install the latest stable release and load its PATH configuration:

```sh
curl -fsSL https://github.com/imrenagicom/adsctl/releases/latest/download/install.sh | sh
source "$HOME/.adsctl/env"
adsctl version
adsctl config path
```

The installer selects the OS and architecture, verifies the archive against the release's SHA-256 checksums, checks that the binary runs and supports `ADSCTL_HOME`, and installs without sudo. It prints the commands to add the generated `env` file to your shell startup file and reload it. For POSIX `sh`, use `. "$HOME/.adsctl/env"` instead of `source`. It does not edit shell startup files automatically.

Everything defaults to one private directory:

```text
~/.adsctl/
├── bin/adsctl        # executable; add this directory to PATH
├── config.yaml       # profiles, Meta API versions, and credential references
├── secrets/          # optional raw-token files, restricted to your user
├── state/            # launch journals and monitoring state
└── env               # shell setup for ADSCTL_HOME and PATH; no credentials
```

Set an absolute `ADSCTL_HOME` to use another directory. The installer and CLI use that same root. Pass `--version vX.Y.Z` to the installer to select a release with `ADSCTL_HOME` support, or set `ADSCTL_VERSION`. Reinstalling replaces the executable and generated shell setup while preserving configuration, secrets, and state. Windows users can extract the matching release ZIP, put `adsctl.exe` in `%USERPROFILE%\.adsctl\bin`, and add that directory to PATH.

The curl endpoint becomes available in the first release that publishes `install.sh`; older release archives remain available for manual installation.

## Codex and Claude Code plugin

The repository is also an `adsctl` plugin for Codex and Claude Code, bundling all three skills. Register a local checkout or extracted plugin distribution and install it:

```sh
# Codex
codex plugin marketplace add /absolute/path/to/adsctl
codex plugin add adsctl@adsctl

# Claude Code
claude plugin marketplace add /absolute/path/to/adsctl
claude plugin install adsctl@adsctl
```

Run the setup workflow in Codex or `/adsctl:adsctl-setup` in Claude Code, then use `/adsctl:adsctl-meta-analyst` for campaign performance audits or `/adsctl:adsctl` for general reporting and management operations. The CLI must be available on the agent host's PATH and Meta credentials must be supplied separately. See [plugin installation and verification](docs/plugins.md) for GitHub installation, development, updates, and platform requirements.

## Build and verify

Requires Go 1.25.1 or later. Persistent launch and monitor state currently uses macOS/Linux file locking. Other commands can compile for Windows; Windows persistent execution is explicitly rejected.

```sh
make tools       # install pinned golangci-lint
make check       # formatting, vet, lint, race tests, coverage and build
./bin/adsctl --help
export PATH="$PWD/bin:$PATH"
# Optional: install into your Go bin directory
make install
```

CI repeats these checks on pull requests and pushes. Tests use local HTTP fixtures, including pagination, lossless IDs, account ownership, exact metrics, write verification, ambiguous outcomes, launch recovery and activation order. No test spends money or calls a live Meta account.

## Distributions and releases

GitHub [releases](https://github.com/imrenagicom/adsctl/releases) contain binaries for Linux, macOS (`darwin`) and Windows, each for x86-64 (`amd64`) and ARM64 (`arm64`). Linux/macOS downloads are `.tar.gz`; Windows downloads are `.zip`. Distributions include both plugin manifests, marketplace catalogs, shared skills, and installation instructions alongside the binary. `checksums.txt` records SHA-256 hashes. Earlier releases may contain only the CLI and skills. Windows persistent launch/monitor execution remains unsupported as described above.

GoReleaser v2.18.3 builds the distributions with CGO disabled and injects the version through `-X adsctl/internal/cli.version=...`. `adsctl version` reports that value in its normal JSON envelope. Local builds default to `dev`; override it with `make build VERSION=0.1.0` or `make install VERSION=0.1.0`.

semantic-release uses the existing `v0.1.0` tag as its baseline. After the Go checks and distribution checks pass, pushes to `main` trigger releases based on Conventional Commits:

| Commit | Release |
| --- | --- |
| `fix: correct report totals` | Patch |
| `feat: add a reporting command` | Minor |
| `feat!: change the output contract` or a `BREAKING CHANGE:` footer | Major |
| `docs:`, `test:`, `chore:`, `ci:` | No release by themselves |

Use a Conventional Commit title when squash-merging a pull request. semantic-release builds the proposed version before creating its tag, then publishes generated release notes, the six archives, checksums, and `install.sh` to GitHub. The release job performs publishing directly, because tags created with `GITHUB_TOKEN` do not trigger another Actions workflow. It uses the built-in token with `contents: write`; no npm publishing or npm token is needed. Repository rules must allow this workflow to create version tags. The CI workflow can also be run manually on `main` to release any unreleased commits.

For local checks, install GoReleaser v2.18.3 and Node.js 24.10 or later:

```sh
npm ci
make release-snapshot   # build all six archives without publishing
# With GitHub credentials available, inspect the next version without publishing:
npm run release:dry-run
```

Generated distributions live in the ignored `dist/` directory. CI uploads snapshot distributions as workflow artifacts for review. The npm package and lockfile only manage release tooling; Git tags remain the source of release versions. If GitHub publishing fails after a version tag is pushed, recover that tagged release with GoReleaser from a clean checkout of that tag (`goreleaser release --clean`, with `GITHUB_TOKEN` set); rerunning semantic-release alone will not republish an existing tag.

## Configure external credentials

Create a Meta developer app, grant it the required Marketing API access and asset permissions, and supply a token through your runner's secret store. The CLI consumes that token; it does not provision the app, perform OAuth or refresh tokens.

```sh
adsctl config profiles set client-read --platform meta \
  --token-env META_CLIENT_READ_TOKEN --api-version v25.0 --mode read

adsctl config profiles set client-write --platform meta \
  --token-file /run/secrets/meta-client-write-token \
  --api-version v25.0 --mode write

adsctl config validate
adsctl meta auth check --profile client-read --account act_123
adsctl meta accounts list --profile client-read --all
```

The config stores references, never token values. A token file contains only the raw token and an optional trailing newline. The default config path is `~/.adsctl/config.yaml`, or `$ADSCTL_HOME/config.yaml`; use `adsctl config path` to inspect it or `--config` to select another file. Relative token-file paths resolve from the selected configuration file's directory, independently of the working directory. Optional `--app-secret-env` enables app-secret proof.

To keep a Meta credential under the same root, save its token privately in `~/.adsctl/secrets/meta-read-token` (file mode 0600), then configure its reference:

```sh
adsctl config profiles set client-read --platform meta \
  --token-file secrets/meta-read-token --api-version v25.0 --mode read
```

An example `~/.adsctl/config.yaml` is:

```yaml
version: 1
default_profile: client-read
profiles:
  client-read:
    platform: meta
    token_file: secrets/meta-read-token
    api_version: v25.0
    mode: read
portfolios: {}
```

**Existing installations:** The new default does not automatically move data from the old `$XDG_CONFIG_HOME/adsctl` or `~/.config/adsctl` configuration directory, or from the old OS user-config state directory (on macOS, `~/Library/Application Support/adsctl/state`). Use `--config` and `--state-dir` to keep using those locations, or move configuration, referenced secret files, and state into the new layout while no adsctl commands are running. Update token-file references as needed. The installer never overwrites an existing `config.yaml`.

Read scope precedence is flags, `ADSCTL_PROFILE`/`ADSCTL_ACCOUNT`, then the configured default profile. Advertising writes require explicit `--profile`, explicit `--account` and a `mode: write` profile. Local mode is an additional guard; Meta enforces actual permissions. Authentication checks confirm the requested account read, not every permission needed by future writes.

## Discover and report

```sh
adsctl commands --platform meta --capability read
adsctl meta schema get campaigns --operation create
adsctl meta campaigns list --profile client-read --account act_123 --all
adsctl meta ads get 456 --profile client-read --account act_123

adsctl meta insights query --profile client-read --account act_123 \
  --level ad --date-preset last_7d --time-increment 1 \
  --fields ad_id,ad_name,spend,impressions,clicks,actions --all

adsctl meta reports summary --profile client-read --account act_123
adsctl meta reports rank --profile client-read --account act_123 \
  --level ad --metric cpa --action-type offsite_conversion.fb_pixel_purchase \
  --min-spend 25.00 --order asc --top 10

adsctl meta insights jobs create --profile client-read --account act_123 \
  --level ad --date-preset last_30d --time-increment 1 --wait --timeout 10m
adsctl meta insights jobs results 789 --profile client-read --account act_123 --all
```

Resource discovery covers accounts, businesses, campaigns, ad sets, ads, creatives, images, videos, Pages, Instagram accounts, pixels, audiences and targeting. Reporting includes native Insights, async jobs, summary, comparison, rankings, breakdowns and CSV/JSONL export. Use `--help` on any command for its actual flags.

Lists default to one page. Check `meta.complete` and the scoped continuation cursor; `--all` remains bounded by `--max-items` and deadlines. Higher-level reports reject incomplete inputs. Raw Insights presets use Meta's semantics; higher-level reports resolve `last_Nd` to completed account-local days. CPA/ROAS requires an explicit action type. Missing metrics and zero denominators remain unavailable.

Portfolios are explicit profile/account pairs in config:

```yaml
version: 1
profiles:
  client-read:
    platform: meta
    token_env: META_CLIENT_READ_TOKEN
    api_version: v25.0
    mode: read
portfolios:
  clients:
    - profile: client-read
      account: act_123
```

```sh
adsctl meta reports summary --portfolio clients --concurrency 2
```

Portfolio reports keep accounts and currencies separate. Export operates on one account at a time. Business account discovery supports `--relationship owned|client`.

## Monitor and inspect changes

```sh
adsctl meta issues list --profile client-read --account act_123
adsctl meta snapshots create --profile client-read --account act_123 --save before.json
adsctl meta snapshots create --profile client-read --account act_123 --save after.json
adsctl meta snapshots diff --before before.json --after after.json
adsctl meta monitor validate --file examples/monitor.yaml
adsctl meta monitor check --file examples/monitor.yaml \
  --profile client-read --account act_123 --state-dir ./private-state
```

Monitoring is one-shot, read-only and deterministic. An external agent or scheduler decides when to run it. Rules support delivery statuses and metric thresholds, currency checks, minimum evidence, conversion delay buffers and persisted started/ongoing/recovered transitions. `--fail-on-finding` adds exit 11. No schedules or notifications are created. Incomplete evidence does not clear prior findings.

## Manage and create

```sh
adsctl meta campaigns pause 456 --profile client-write --account act_123 --dry-run
adsctl meta campaigns pause 456 --profile client-write --account act_123
adsctl meta adsets budget set 789 --profile client-write --account act_123 \
  --daily-budget-minor 2500 --currency USD
adsctl meta campaigns create --profile client-write --account act_123 \
  --name 'Website traffic' --objective OUTCOME_TRAFFIC --special-ad-categories NONE
adsctl meta adsets create --profile client-write --account act_123 --file examples/adset.json
```

Campaigns, ad sets, ads and creatives support create, update, local validation and explicit delete. Campaigns/ad sets/ads also support pause, activate and archive. Previews, creative replacement, image/video uploads and video processing checks are available. Video uploads default to checkpointed resumable mode; use `--resume <upload_id>` with the same file/profile/account after interruption or `--mode single` for one multipart request. Creates default to `PAUSED`; an individual create can explicitly request `ACTIVE`. Budget convenience commands check account currency, budget ownership and existing budget type. Native payload files use native minor units without currency conversion.

`--file` accepts native JSON, including `-` for stdin, and cannot be mixed with scalar write flags. The mutation schema deliberately covers a subset of Meta fields; inspect it with `meta schema get`. Nested creative/targeting conditions remain Meta's responsibility. Validation commands perform local schema and account/parent-access checks; they **do not invoke Meta's validate-only mode** or guarantee acceptance. Read-after-write receipts preserve the observed fields. Acknowledgment, configured status and actual delivery are distinct.

## Launch complete campaigns

```sh
adsctl meta launch template --objective traffic --format image --save launch.yaml
# Edit Page ID, media, destination, targeting, budget and native payloads.
adsctl meta launch plan --file launch.yaml --save plan.json \
  --profile client-write --account act_123
adsctl meta launch apply --plan plan.json --profile client-write --account act_123
adsctl meta launch status RUN_ID
adsctl meta launch resume RUN_ID --profile client-write --account act_123
adsctl meta launch activate RUN_ID --profile client-write --account act_123 --dry-run
adsctl meta launch activate RUN_ID --profile client-write --account act_123
```

The starter template supports website traffic with an image. Native manifests support the compiled resource schemas, existing resource references, image references and video references (`object_story_spec.video_data.video_id`). Assets resolve relative to the manifest. Plans record scope, currency, payloads, dependencies and file hashes. Apply revalidates them, creates paused resources and checkpoints IDs. Reapplying the same plan in the same state directory is rejected; use its recorded run. A new plan or a different state directory does not provide remote deduplication.

Interrupted unknown creates require explicit reconciliation:

```sh
adsctl meta launch reconcile RUN_ID --step campaigns.main --remote-id 456 \
  --profile client-write --account act_123
# Or explicitly record a decision to retry, accepting possible duplication:
adsctl meta launch reconcile RUN_ID --step campaigns.main --retry-unknown \
  --profile client-write --account act_123
```

Reconciliation checks account membership and planned fields; image attachment checks the hash exists in the selected account. Resume does not recreate known IDs. Video processing can interrupt a run with its known video ID; wait, then resume. Activation touches only recorded run objects, children before parents. Existing active parents can allow delivery as soon as a child is activated. Repeating `launch activate` checks configured status before continuing; activation success does not prove delivery. Failed runs are never automatically deleted.

Local state defaults to `~/.adsctl/state`, or `$ADSCTL_HOME/state`; set `--state-dir` consistently for runners that need another location. State files are private and writes are atomic. Locks coordinate processes on one host, not distributed runners.

## Agent contract and current limits

Data-producing commands emit a versioned envelope with `ok`, native `data`, scope/query/completeness `meta`, `warnings` and `error`. JSONL uses meta/data/error/end records; results are currently buffered within configured bounds, not streamed while fetching. Tables are generic, lossy and have no ANSI colors. Execution errors use JSON on stderr; partial operations may also emit an `ok:false` receipt on stdout. Help, completion and generated Markdown docs are text.

Exit codes: 0 success; 1 internal; 2 usage/config; 3 credentials; 4 permission/scope; 5 validation; 6 throttling; 7 timeout; 8 provider/network; 9 incomplete or applied-unverified; 10 unknown mutation outcome; 11 explicit monitoring findings. Unknown mutations are not automatically retried. GET retries are bounded, honor deadlines and pace on exposed usage/Retry-After headers.

**Live Meta validation remains outstanding.** No credentials were supplied, so fixtures establish CLI behavior, not live account compatibility. v25.0 is the initial compatibility target; version overrides are exposed but this is not a tested cross-version matrix. Account-specific permissions, objectives, breakdown availability, special categories, creative requirements and normalized response shapes still require live smoke testing. Google, automatic optimization, token refresh, remote validation-only execution, distributed locking and built-in scheduling are not implemented. No command automatically applies provider recommendations.

See [the original architecture](docs/design.md), [the full UX specification](docs/cli-reference.md), [implementation status and verification](docs/implementation.md), [Meta research](docs/meta-research.md), [glossary](CONTEXT.md), and [output ADR](docs/adr/0001-agent-output-contract.md).

## License

This project is licensed under the [MIT License](LICENSE).

