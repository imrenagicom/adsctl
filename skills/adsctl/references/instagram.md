# Instagram publishing

`adsctl ig` publishes organic content to an Instagram professional account through the Instagram Graph API (Facebook Login path: `graph.facebook.com` and `rupload.facebook.com`). It uses the same profiles as `adsctl meta`; the token must hold `instagram_basic` and `instagram_content_publish` for the account, and `instagram_manage_contents` to delete media. `ig auth check` lists any `missing_permissions`. Scope is an IG user ID, not an ad account.

## Find scope and check access

```sh
adsctl ig accounts list --profile "$PROFILE"
adsctl ig auth check --profile "$WRITE_PROFILE" --ig-user "$IG_USER"
adsctl ig limit --profile "$PROFILE" --ig-user "$IG_USER"
adsctl ig media list --profile "$PROFILE" --ig-user "$IG_USER" --page-size 10
```

`accounts list` returns Pages with their `instagram_business_account`; use that `id` as `--ig-user`. `auth check` reports granted permissions when the token allows `me/permissions`; `content_publish_permission: "unknown"` is not a grant.

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
- Publish states: `published` (with `media_id` and `media.permalink` when readable), `already_published` (refused; nothing posted again), `not_ready`, `quota_exhausted`, `unknown`.
- `unknown` (exit 10) means the publish acknowledgment was lost and the container did not yet report `PUBLISHED`. Do not publish again; read `adsctl ig containers get CONTAINER_ID` and `ig media list` first. `published` with `media_id: null` means the container confirmed `PUBLISHED` after a lost acknowledgment.
- Video processing can take minutes. Waiting commands default to a 10-minute `--timeout`; exit 7 leaves a valid container to `containers wait` and publish.

Report the IG user, container ID, media ID, permalink and caption source. Record the published media ID with the campaign's post record so later reporting and ad boosts (`source_instagram_media_id`) reference it.
