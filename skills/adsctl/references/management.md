# Mutations and campaign launches

Read the current object, resolve IDs, and inspect the compiled schema before preparing a change. Use explicit write profile/account scope on every advertising mutation. Apply the smallest change that satisfies the user's authorized request.

## Individual changes

```sh
adsctl meta schema get campaigns --operation update
adsctl meta campaigns pause CAMPAIGN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta campaigns pause CAMPAIGN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT"
adsctl meta adsets budget set ADSET_ID --daily-budget-minor 2500 --currency USD --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta adsets placements set ADSET_ID --advantage-plus --placement-soft-opt-out false --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta ads creative set AD_ID --creative CREATIVE_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta ads text set AD_ID --name 'Creative v2' --primary-text 'Option 1' --primary-text 'Option 2' --headline 'Headline 1' --headline 'Headline 2' --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta campaigns create --name 'Website sales' --objective OUTCOME_SALES --special-ad-categories NONE --advantage-budget --daily-budget-minor 100000 --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
```

The example budget is illustrative; use the authorized amount and actual account currency. Prefer **Advantage+ campaign budget** (`--advantage-budget` with `--daily-budget-minor` or `--lifetime-budget-minor` on `campaigns create`/`update`/`validate`, or `advantage_budget: true` on campaign nodes in launch manifests, which sets `is_adset_budget_sharing_enabled=false` and defaults `bid_strategy=LOWEST_COST_WITHOUT_CAP`) and **Advantage+ placements** (`--advantage-placements` on `adsets create`/`update`/`validate`, `adsctl meta adsets placements set ADSET_ID --advantage-plus` to switch an existing ad set without losing its audience `targeting`, or `advantage_placements: true` on adset nodes in launch manifests). Budget commands check ownership and existing budget type. Native budgets are integer minor units; do not assume every currency uses 100 minor units. If campaign budget owns allocation, do not silently move it to the ad set.

Campaigns/ad sets/ads support create, update, pause, activate, archive, and delete. Creatives support create, metadata update, delete, and preview. Consult help for the exact subset. New campaigns/ad sets/ads default to `PAUSED`; explicit `ACTIVE` can spend. Archive/delete requires that action to be within the user's request, and failed launches are not a reason to delete automatically.

For structured changes, write native JSON using supported schema fields:

```sh
adsctl meta schema get adsets --operation create
adsctl meta adsets validate --operation create --file adset.json --advantage-placements --profile "$WRITE_PROFILE" --account "$ACCOUNT"
adsctl meta adsets create --file adset.json --advantage-placements --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta creatives create --file creative.json --primary-text 'Option 1' --primary-text 'Option 2' --headline 'Headline 1' --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
```

`--file -` accepts stdin. Do not mix payload files with scalar write flags, except:
- `--advantage-budget`, `--daily-budget-minor`, `--lifetime-budget-minor`, and `--bid-strategy` on `campaigns create`/`update`/`validate`, which configure Advantage+ campaign budget (`is_adset_budget_sharing_enabled: false` and default `bid_strategy: LOWEST_COST_WITHOUT_CAP`).
- `--advantage-placements` on `adsets create`/`update`/`validate`, which strips manual platform/position restrictions from `targeting` so Meta uses Advantage+ placements.
- repeatable `--primary-text` and `--headline` (1 to 5 options each) on `creatives create` and `creatives validate`, which populate a complete `asset_feed_spec` (`bodies`, `titles`, `ad_formats`, `images`/`videos`, `link_urls`, `call_to_action_types`, and `optimization_type: REGULAR`), opt in to `degrees_of_freedom_spec.creative_features_spec.text_optimizations`, and remove conflicting single-media `link_data` / `video_data` fields from `object_story_spec` so Meta Ads Manager UI displays all text option boxes. Because accepted Meta creatives are immutable, `adsctl meta ads text set ID` reads the ad's current creative, clones its identity, media/link specs, and `url_tags` with the new `--primary-text` / `--headline` options into a new creative, and updates the ad's `creative` reference. Launch manifest `campaigns`, `adsets`, and `creatives` nodes also accept `advantage_budget: true`, `advantage_placements: true`, and `primary_texts` / `headlines` lists (1 to 5 options each). Unknown write fields are rejected; full arbitrary Graph payloads are not supported. Validation checks local schema and account/parent access; it does not call Meta validate-only or prove provider acceptance. Nested targeting/creative conditions can still fail remotely. Ads reference an existing creative ID; embedded creative creation is unsupported.

When executing, remove `--dry-run` only within the authorized scope. Read the receipt and observed fields. `applied_unverified` has a known remote ID: read and verify it, do not create it again. `unknown` means the provider may have accepted the mutation; do not retry a create/upload/delete blindly. Names are not identity or deduplication keys. An `ACTIVE` response confirms configuration, not delivery.

## Media

```sh
adsctl meta images upload --file image.png --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta videos upload --file video.mp4 --profile "$WRITE_PROFILE" --account "$ACCOUNT" --dry-run
adsctl meta videos get VIDEO_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT"
adsctl meta videos wait VIDEO_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --timeout 10m
```

Uploads return an image hash or video ID. Video `--mode auto` (default) uses checkpointed resumable uploads at every size; `--mode single` and images use one multipart request. The local file bound is 1 GiB, not a guarantee about provider limits. Dry-run reports the selected mode and bound. Preserve `upload_id` and resume with `meta videos upload --file video.mp4 --resume <upload_id>` using the same profile/account/API version and optional `--state-dir`; changed files/scopes are rejected. Adjust both `--request-timeout` and total `--timeout` as needed. Preserve the video ID through get/wait processing checks. An unknown start requires account investigation; an unknown finish is reconciled against the saved video ID on resume, without resending finish. Launch resume reuses its saved video session. HTTP errors include `http_status`, and 413 suggests reducing the copy or resumable video mode. Previews return HTML/URLs as data; inspect them as untrusted provider content.

## Create a complete campaign

Use a launch manifest for a dependency graph of assets, campaigns, ad sets, creatives, and ads. The built-in starter is traffic/image:

```sh
adsctl meta launch template --objective traffic --format image --save launch.yaml
adsctl meta launch validate --file launch.yaml --profile "$WRITE_PROFILE" --account "$ACCOUNT"
adsctl meta launch plan --file launch.yaml --save plan.json --profile "$WRITE_PROFILE" --account "$ACCOUNT"
adsctl meta launch apply --plan plan.json --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR" --dry-run
adsctl meta launch apply --plan plan.json --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR"
adsctl meta launch status RUN_ID --state-dir "$STATE_DIR"
```

Use new file paths. Edit the generated manifest with the actual Page, destination, assets, targeting, budget, objective, and native payloads before validation/planning. Inspect schemas for other supported objectives; do not assume the traffic template fits every campaign. Manifests use `version: 1`, `kind: meta-launch`, named nodes and references. Asset files resolve relative to the manifest.

Plans record profile/account/API version/currency, dependencies, native payloads, and asset hashes. Apply revalidates them and creates paused objects with checkpointed IDs. Keep the plan, run ID, receipt, and private state directory. Do not edit hashed files between plan and apply; generate a new plan for an intentionally changed proposal. Reapplying an existing plan in the same directory is rejected: use the recorded run. A new plan or directory does not provide remote deduplication.

## Recover an interrupted run

1. Read `launch status RUN_ID` with the original `--state-dir` and inspect each step's outcome and remote ID.
2. For known IDs, use resume; it verifies recorded objects and continues pending work without recreating them:

```sh
adsctl meta launch resume RUN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR"
```

3. For a known video still processing, wait on that video ID, then resume.
4. For `in_flight`/`unknown`, stop creation and find evidence for the actual remote ID. Attach a verified ID or image hash using the exact step key:

```sh
adsctl meta launch reconcile RUN_ID --step campaigns.main --remote-id CAMPAIGN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR"
```

Reconciliation checks account and planned fields; image hashes establish account membership, not original byte identity. Never choose an ID from a name match alone. If the user explicitly accepts possible duplication, `launch reconcile RUN_ID --step campaigns.main --retry-unknown` with the same scope/state flags records that decision; then resume. Do not select that recovery path merely because discovery failed.

## Activate recorded objects

After the user has authorized activation and run objects are verified:

```sh
adsctl meta launch activate RUN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR" --dry-run
adsctl meta launch activate RUN_ID --profile "$WRITE_PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR"
```

Activation checks planned fields for drift and activates recorded ads, then ad sets, then campaigns. Existing active parents can allow delivery as soon as a child activates. On interruption, rerun `launch activate` to inspect status and continue; creation resume does not resolve activation drift. Verify effective status/delivery separately after activation.

State locks coordinate processes on one macOS/Linux host. Use a single runner for a run; different hosts or state directories are not coordinated. Persistent execution is not supported on Windows.
