# Instagram publishing and insights

`adsctl ig` publishes organic content to an Instagram professional account through the Instagram Graph API (Facebook Login path: `graph.facebook.com` and `rupload.facebook.com`). It uses the same profiles as `adsctl meta`; the token must hold `instagram_basic` and `instagram_content_publish` for the account, `instagram_manage_contents` to delete media, and `instagram_manage_insights` to read insights. `ig auth check` lists any `missing_permissions`. Scope is an IG user ID, not an ad account.

## Find scope and check access

```sh
adsctl ig accounts list --profile "$PROFILE"
adsctl ig auth check --profile "$WRITE_PROFILE" --ig-user "$IG_USER"
adsctl ig limit --profile "$PROFILE" --ig-user "$IG_USER"
adsctl ig media list --profile "$PROFILE" --ig-user "$IG_USER" --page-size 10
```

`accounts list` returns Pages with their `instagram_business_account`; use that `id` as `--ig-user`. `auth check` reports granted permissions when the token allows `me/permissions`; `content_publish_permission: "unknown"` is not a grant.

## Insights

Insights are reads and work with a read profile. They cover organic performance; paid delivery of a boosted post belongs to `adsctl meta insights`.

```sh
adsctl ig insights media --profile "$PROFILE" --ig-user "$IG_USER" --max-items 25
adsctl ig insights media --profile "$PROFILE" --ig-user "$IG_USER" --type reels --since 2026-10-01 --until 2026-10-31
adsctl ig insights media MEDIA_ID [MEDIA_ID...] --profile "$PROFILE"
adsctl ig insights media --profile "$PROFILE" --ig-user "$IG_USER" --stories
adsctl ig insights account --profile "$PROFILE" --ig-user "$IG_USER" --date-preset last_28d
adsctl ig insights account --profile "$PROFILE" --ig-user "$IG_USER" --metric reach --metric-type time_series --since 2026-10-01 --until 2026-10-28
adsctl ig insights audience --profile "$PROFILE" --ig-user "$IG_USER" --audience followers --breakdown age,gender
```

- `media` returns one row per item: `id`, `media_product_type`, `media_type`, `permalink`, `timestamp`, then each metric by its Meta name. Default metrics by product type: FEED `reach,views,likes,comments,shares,saved,total_interactions,follows,profile_visits`; REELS `reach,views,likes,comments,shares,saved,total_interactions,ig_reels_avg_watch_time,ig_reels_video_view_total_time`; STORY `reach,views,replies,shares,total_interactions,follows,profile_visits,navigation`. An explicit `--metric` is sent unchanged for every item, so keep a mixed selection to metrics all product types support, or filter with `--type`. `--breakdown action_type` (with `profile_activity`) and `story_navigation_action_type` (with `navigation`) add `<metric>_by_<dimension>` maps. `--raw` adds Meta's native rows under `insights`.
- Listing without IDs reads the newest `--max-items` media and keeps those published within `--since`/`--until`; raise `--max-items` for older periods and check `meta.media_complete`. Ad-only media (`AD`) and carousel children have no organic insights. Story insights exist only while the Story is live (24 hours), so collect them the same day.
- A failed item gets `error: {code, message}` on its row with a `PARTIAL_INSIGHTS` warning; `Not enough viewers` (`ACCESS_DENIED`) is normal for Stories under 5 viewers. Invalid tokens, throttling, timeouts and missing permissions stop the run and return the rows read so far.
- `account` defaults to `total_value` over `last_7d` (complete UTC days; `meta.range` records the exact window). `--breakdown media_product_type|follow_type|follower_type|contact_button_type` works only with `total_value`; `time_series` returns one row per `end_time` and Meta supports it for `reach` only; Meta's daily buckets end at midnight US Pacific (`end_time` 07:00 or 08:00 UTC), so they do not align exactly with the UTC request window. Account totals include paid delivery: use `--breakdown media_product_type` and read the `AD` share before calling account reach or views organic. `reach` and `accounts_engaged` are deduplicated estimates: do not add daily values or per-post reach to get account reach.
- `audience` needs at least 100 followers (or 100 engagements in the timeframe for `--audience engaged`), returns only Meta's top 45 values, and uses `--timeframe this_week|this_month`.
- Missing metrics mean Meta returned no data, not zero. Values can lag by up to 48 hours, so treat the last two days as provisional.

## Publish

Publishing makes content public, and the Graph API offers no reliable unpublish. Treat it like activation: prepare the exact media, caption and account, run `--dry-run`, and publish only with the user's approval of that plan.

```sh
adsctl ig post --profile "$WRITE_PROFILE" --ig-user "$IG_USER" --type reel --file reel.mp4 --caption-file caption.md --dry-run
adsctl ig post --profile "$WRITE_PROFILE" --ig-user "$IG_USER" --type reel --file reel.mp4 --caption-file caption.md
```

Types and media sources:

| `--type` | Source | Extra flags |
|---|---|---|
| `image` | `--image-url` (public JPEG) | `--caption`, `--alt-text`, `--location-id`, `--collaborator` |
| `reel` | `--file` (.mp4/.mov) or `--video-url` | `--caption`, `--cover-url`, `--thumb-offset`, `--share-to-feed`, `--audio-name`, `--location-id`, `--collaborator` |
| `story` | `--image-url`, `--video-url` or `--file` | none (stories have no caption) |
| `carousel` | 2 to 20 `--item image=URL` / `--item video=URL` in order | `--caption`, `--location-id`, `--collaborator` |

Local files are accepted only for Reel and Story video. Images must already be hosted as JPEG at a public URL; convert PNG/WebP first. `--caption-file -` reads stdin. Captions are limited locally to 2,200 characters, 30 hashtags and 20 mentions.

To stage first and publish later (for example after review), use `--no-publish`, or the step commands:

```sh
adsctl ig containers create --profile "$WRITE_PROFILE" --ig-user "$IG_USER" --type reel --file reel.mp4 --caption-file caption.md
adsctl ig containers wait CONTAINER_ID --profile "$PROFILE"
adsctl ig publish CONTAINER_ID --profile "$WRITE_PROFILE" --ig-user "$IG_USER" --dry-run
adsctl ig publish CONTAINER_ID --profile "$WRITE_PROFILE" --ig-user "$IG_USER"
```

Carousels built by hand: create each child with `--type carousel-item`, then `containers create --type carousel --child ID --child ID`.

## Delete

```sh
adsctl ig media delete MEDIA_ID --profile "$WRITE_PROFILE" --ig-user "$IG_USER" --dry-run
adsctl ig media delete MEDIA_ID --profile "$WRITE_PROFILE" --ig-user "$IG_USER"
```

Deletion is permanent and covers non-ad posts, Reels, Stories and whole carousels (not single carousel items). The command first verifies the media belongs to `--ig-user` (`ACCOUNT_MISMATCH` otherwise). Delete only media the user explicitly named, such as a test post created in the same task; never infer deletion from a cleanup request. An unknown delete is reconciled by reading the media: a not-found read means it was deleted.

## Interpret receipts

- Container states: `planned`, `created`, `uploaded`, `created_upload_failed`, `ready`, `processing` (timeout; wait again), `processing_failed`. Containers are private and expire after 24 hours, so a failed or unknown container step is safe to repeat; nothing was published.
- A local `--file` upload that Meta rejects (often HTTP 400 with `debug_info` `ProcessingFailedError: Request processing failed`, marked not retriable, although the same file is accepted minutes later) is retried on a new private container: `--upload-attempts` (default 3; 1 disables) and `--upload-retry-wait` (default 1m, doubling). Each try is listed in the receipt's `upload_attempts`; only the container that uploaded is waited on and published. Credential, permission, input and deadline errors are not retried. The error message carries Meta's `debug_info` type and message.
- Publish states: `published` (with `media_id` and `media.permalink` when readable), `already_published` (refused; nothing posted again), `not_ready`, `quota_exhausted`, `unknown`.
- `unknown` (exit 10) means the publish acknowledgment was lost and the container did not yet report `PUBLISHED`. Do not publish again; read `adsctl ig containers get CONTAINER_ID` and `ig media list` first. `published` with `media_id: null` means the container confirmed `PUBLISHED` after a lost acknowledgment.
- Video processing can take minutes. Waiting commands default to a 10-minute `--timeout`; exit 7 leaves a valid container to `containers wait` and publish.

Report the IG user, container ID, media ID, permalink and caption source. Record the published media ID with the campaign's post record so later reporting and ad boosts (`source_instagram_media_id`) reference it.
