---
name: adsctl-meta-analyst
description: "Audit and evaluate Meta advertising campaign performance using adsctl. Run structured diagnostic checks across delivery health, creative hook and video retention, click efficiency, post-click landing page and tracking continuity, placement breakdowns, and conversion economics, then produce evidence-backed recommendations. Use when the user asks how a Meta campaign or ad account is performing, wants a media-buyer audit, asks whether to pause, hold, or scale ads, or needs a diagnostic performance review."
---

# adsctl Meta Analyst

Evaluate live Meta advertising performance like an experienced performance marketing analyst using read-only `adsctl` queries. Treat metrics as a chronological funnel and system diagnostic rather than isolated numbers.

## 1. Establish scope, objective, and campaign maturity

1. Resolve `adsctl` on PATH (or `bin/adsctl` in a checkout). Identify the read `--profile`, `--account` (`act_...`), target campaign/ad set/ad IDs, and requested reporting window. If missing, inspect `adsctl config profiles list` and `adsctl meta accounts list --profile "$PROFILE" --all`. Never switch accounts silently.
2. Inspect the structural configuration and delivery health before reading performance numbers:

```sh
adsctl meta accounts get "$ACCOUNT" --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta issues list --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta campaigns list --profile "$PROFILE" --account "$ACCOUNT" --all
adsctl meta campaigns get "$CAMPAIGN_ID" --fields id,name,objective,status,effective_status,daily_budget,lifetime_budget,bid_strategy,start_time,stop_time --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta adsets list --campaign "$CAMPAIGN_ID" --profile "$PROFILE" --account "$ACCOUNT" --all
adsctl meta adsets get "$ADSET_ID" --fields id,name,status,effective_status,optimization_goal,billing_event,daily_budget,lifetime_budget,promoted_object,destination_type,targeting,start_time,end_time --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta ads list --campaign "$CAMPAIGN_ID" --profile "$PROFILE" --account "$ACCOUNT" --all
```

3. Classify **campaign maturity** from `start_time`, total spend, impressions, and optimization events:
   - **Launch / Calibration (`< 48h` or `< 1,500` impressions or `< 1x` daily budget spent):** Evaluate delivery, pacing, CPM, thumbstop/hold rate, outbound CTR, and tracking continuity. Do not judge conversion rate (CVR), CPA, or ROAS as failed yet.
   - **Learning Phase (`< 7d` or `< 50` optimization events per ad set in 7 days):** Evaluate directional creative separation and mid-funnel signals; avoid recommending disruptive edits that reset learning unless a variant is severely lagging after meaningful spend.
   - **Established (`>= 7d` and sufficient spend/conversions):** Evaluate full-funnel unit economics (CPA, ROAS, CVR), period-over-period trends, frequency saturation, and scaling readiness.

## 2. Pull the multi-layer diagnostic evidence

Query complete (`--all`) JSON envelopes. Check exit code, `ok`, and `meta.complete` on every call.

### Campaign and ad-level full-funnel metrics

```sh
adsctl meta insights query --campaign "$CAMPAIGN_ID" --level campaign --date-preset maximum \
  --fields campaign_id,campaign_name,spend,impressions,reach,frequency,clicks,ctr,cpc,cpm,inline_link_clicks,inline_link_click_ctr,cost_per_inline_link_click,outbound_clicks,outbound_clicks_ctr,cost_per_outbound_click,unique_clicks,unique_ctr,unique_inline_link_clicks,unique_inline_link_click_ctr,video_thruplay_watched_actions,video_p25_watched_actions,video_p50_watched_actions,video_p75_watched_actions,video_p95_watched_actions,video_p100_watched_actions,video_avg_time_watched_actions,actions,action_values,cost_per_action_type,purchase_roas \
  --all --profile "$PROFILE" --account "$ACCOUNT"

adsctl meta insights query --campaign "$CAMPAIGN_ID" --level ad --date-preset maximum \
  --fields ad_id,ad_name,adset_id,adset_name,spend,impressions,reach,frequency,clicks,ctr,cpc,cpm,inline_link_clicks,inline_link_click_ctr,cost_per_inline_link_click,outbound_clicks,outbound_clicks_ctr,cost_per_outbound_click,video_thruplay_watched_actions,video_p25_watched_actions,video_p50_watched_actions,video_p75_watched_actions,video_p95_watched_actions,video_p100_watched_actions,video_avg_time_watched_actions,quality_ranking,engagement_rate_ranking,conversion_rate_ranking,actions,action_values,cost_per_action_type,purchase_roas \
  --all --profile "$PROFILE" --account "$ACCOUNT"
```

### Daily trajectory, placement breakdown, and creative inspection

```sh
# Separate completed account-local days from partial current-day delivery
adsctl meta insights query --campaign "$CAMPAIGN_ID" --level campaign --date-preset maximum --time-increment 1 \
  --fields campaign_id,campaign_name,spend,impressions,reach,clicks,ctr,inline_link_clicks,inline_link_click_ctr,outbound_clicks,outbound_clicks_ctr,actions,action_values \
  --all --profile "$PROFILE" --account "$ACCOUNT"

# Placement efficiency (Feed vs Stories vs Reels)
adsctl meta insights query --campaign "$CAMPAIGN_ID" --level campaign --breakdowns publisher_platform,platform_position --date-preset maximum \
  --fields spend,impressions,clicks,ctr,inline_link_clicks,inline_link_click_ctr,outbound_clicks,outbound_clicks_ctr,actions,action_values \
  --all --profile "$PROFILE" --account "$ACCOUNT"

# Inspect ad creatives (CTA type, destination URLs, copy, video/image assets)
adsctl meta ads get "$AD_ID" --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta creatives get "$CREATIVE_ID" --profile "$PROFILE" --account "$ACCOUNT"
```

When the campaign has at least 7–14 completed days of history, also run period-over-period comparisons (`adsctl meta reports compare --campaign "$CAMPAIGN_ID" --level ad --date-preset last_7d --compare previous-period --fields spend,impressions,clicks,ctr,inline_link_clicks,inline_link_click_ctr,actions,action_values --profile "$PROFILE" --account "$ACCOUNT"`). Use `--include-partial-period` on `reports` commands only when intentionally including today.

## 3. Analyze the 5-stage funnel in order

Read [metrics-and-benchmarks.md](references/metrics-and-benchmarks.md) for exact formulas, Meta `actions` keys, and benchmark ranges. Evaluate every campaign through five sequential stages:

1. **Delivery, Pacing & Saturation:** Compare daily spend against configured budget (CBO vs ABO). Check `CPM`, `Frequency` (`Impressions / Reach`), and **First-Time Impression Ratio** (`Reach / Impressions`). Inspect `quality_ranking`, `engagement_rate_ranking`, and `conversion_rate_ranking` (noting `UNKNOWN` is normal below ~500–1,000 impressions).
2. **Creative Hook & Video Retention:** Compute **Thumbstop (3s Hook) Rate** (`actions[video_view] / impressions`), **ThruPlay Rate** (`video_thruplay_watched_actions / impressions`), **Hold Rate** (`ThruPlay / 3s video_view` and `p25 -> p50 -> p75 -> p100` survival), average watch time, and **Social Proof Velocity** (reactions, saves, shares, comments per 1,000 impressions).
3. **Click Efficiency & Intent:** Always separate **All Clicks / All CTR** (includes profile taps, "See more", reactions, video taps) from **Link / Outbound CTR** (`inline_link_click_ctr`, `outbound_clicks_ctr`) and **Cost per Outbound Click** (`cost_per_outbound_click`). Check Unique Link CTR (`unique_inline_link_click_ctr`) and how CBO distributes spend across sibling ads.
4. **Post-Click Bridge & Tracking Continuity:** Compare `outbound_clicks` to `landing_page_view` (`Outbound-to-LPV Ratio = landing_page_view / outbound_click`) and mid-funnel events (`view_content`, `add_to_cart`, `initiate_checkout`, `lead`). If outbound clicks exist but `landing_page_view` and mid-funnel pixel events are missing, determine whether the site has a tracking/consent/redirect drop-off or intentionally uses server-side CAPI only on final checkout.
5. **Conversion Economics:** Evaluate target conversions (`purchase`, `lead`, `complete_registration`), **Click-to-Conversion Rate (CVR)**, **CPA** (`spend / conversions`), **ROAS** (`action_values / spend`), and **AOV** (`action_values / purchases`) against the ad set's `optimization_goal` and attribution setting.

## 4. Diagnose bottlenecks and formulate recommendations

Read [diagnostic-playbook.md](references/diagnostic-playbook.md) to map funnel symptoms to root causes and decision rules.

- **Respect sample-size gates:** Never declare a conversion campaign unprofitable or recommend pausing an ad on a handful of clicks unless there is a broken destination link, policy rejection (`DISAPPROVED` / `WITH_ISSUES`), or severe top-of-funnel failure after meaningful impressions (`>= 1,500–2,500` impressions).
- **Isolate the bottleneck stage:** Distinguish a weak first-3-second hook (low Thumbstop) from weak body pacing (normal Thumbstop, steep `p25 -> p50` drop), weak offer/CTA framing (normal retention, low Outbound CTR), or post-click landing-page/checkout friction (healthy Outbound CTR, low LPV or 0 conversions after sufficient clicks).
- **Classify every recommendation with concrete checkpoints:** Label each action as **Hold (Keep Learning)**, **Watch (Checkpoint at X spend/impressions/days)**, **Creative/Copy Iteration**, **Landing Page / Tracking Check**, **Pause Variant**, or **Scale / Consolidate Budget**.
- **Preserve read-only safety:** Performance analysis never authorizes mutating live campaigns, budgets, or statuses. If the user later asks to apply a recommended change or set up automated rules (`adsctl meta monitor`), transition to the `adsctl` skill and follow its explicit write/monitoring procedures.

## 5. Present the analyst report

Structure the response clearly for decision-making:

1. **Executive Verdict:** 2–3 sentences stating campaign maturity, overall funnel health, and the primary recommendation (e.g., hold, iterate hook, pause laggard at checkpoint, or scale).
2. **Funnel Scorecard:** Table of Campaign Totals and Daily Pacing (separating completed days from partial today) covering Spend, Impressions, Reach, Frequency, CPM, Thumbstop/Hold (for video), All CTR, **Outbound/Link CTR**, **Cost per Outbound Click**, Mid-Funnel/LPV, and Target Conversions/CPA/ROAS.
3. **Ad Variant & Placement Breakdown:** Side-by-side comparison of active ads/ad sets and placements (`instagram_reels`, `instagram_stories`, `feed`, etc.), highlighting where CBO/ABO is routing spend and why.
4. **Strengths, Bottlenecks & Tracking Notes:** Evidence-backed diagnosis of what is working, where the funnel leaks, and any attribution or pixel/CAPI observability gaps.
5. **Action Plan & Next Checkpoints:** Concrete spend, impression, or date thresholds for the next decision.
6. **Scope & Attribution Footer:** Profile, ad account ID, currency, timezone, API version, date range, attribution mode/report time, and `meta.complete` verification.
