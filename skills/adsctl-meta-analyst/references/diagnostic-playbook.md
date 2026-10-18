# Meta Campaign Diagnostic Playbook & Recommendation Rules

Use this reference to translate observed funnel metrics into root-cause diagnoses, sample-size-aware judgments, and concrete recommendations.

---

## 1. Sample-Size & Maturity Gates (When to Judge vs. When to Wait)

Prematurely pausing ads or editing ad sets is one of the most common media-buying errors on Meta. Apply these evidence gates before recommending any pause or budget change:

| Campaign / Ad Stage | Typical Evidence Footprint | What You CAN Judge Reliably | What You MUST NOT Do Yet |
| :--- | :--- | :--- | :--- |
| **Gate 0: Pre-Flight / Broken Delivery** | `0` impressions after several hours, or `DISAPPROVED` / `WITH_ISSUES` / account status `!= 1` | Account billing/status (`adsctl meta issues list`), parent campaign/ad set paused status, schedule (`start_time` in future / `stop_time` passed), or broken destination URL. | Do not diagnose creative quality when the ad is not delivering. |
| **Gate 1: Day-1 / Early Launch** | `< 24–48h` live, `< 1,500` impressions per ad, or `< 1x` daily budget spent | Delivery health, CPM range, Thumbstop rate (`3s views / impr`), early Outbound CTR separation between sibling ads (`>= 1,000` impr each), and post-click tracking verification. | **Do not pause ads for 0 conversions.** Do not edit targeting or budgets (resets early learning). |
| **Gate 2: Early Creative & Traffic Validation** | `1,500 – 4,000` impressions per ad (`~50 – 150` outbound clicks) or `1x – 2x` target CPA spent per ad | Statistically meaningful Outbound CTR and Cost per Outbound Click gaps between variants (e.g., Ad A at `1.6%` Link CTR vs Ad B at `0.8%` Link CTR after `2,000+` impressions each); LPV connect rate; mid-funnel `InitiateCheckout`/`Lead` signals. | Do not declare the overall campaign unprofitable if total spend is still `< 2x–3x` target CPA or `< 100` outbound clicks. |
| **Gate 3: Conversion & Unit-Economic Validation** | Spend `>= 2x – 3x` target CPA with `0` conversions, OR `>= 5–10` conversions per variant across `>= 3–7` completed days | True CPA, CVR, ROAS, AOV, placement profitability, and whether a creative angle meets unit economics. | Do not scale budget aggressively (`> 20%` at once) if CPA is above target or the ad set is unstable. |
| **Gate 4: Learning Phase Exit & Scale** | `>= 50` optimization events per ad set within 7 days, stable CPA/ROAS over `>= 7` completed days | Scaling readiness (15–20% budget increments every 48–72h or horizontal creative expansion), audience saturation (`Frequency`, `FTIR`), and 7d-vs-previous-7d trends. | Do not let cold prospecting `Frequency` climb past `2.8–3.0+` without queuing fresh creative hooks. |

---

## 2. The Bottleneck Decision Matrix (Symptom -> Root Cause -> Action)

Walk the funnel top-to-bottom. Fix the **earliest** broken stage first—downstream metrics cannot stabilize until upstream bottlenecks are resolved.

### Pattern A: High CPM + Low Delivery / Under-Pacing
- **Observed Signals:** Daily spend lags far behind configured budget; CPM is unusually high for the geography/vertical; `Frequency` climbs quickly even at low spend.
- **Root Causes:**
  1. Audience is too narrow (e.g., stacked interests, narrow age/geo, or `advantage_audience: 0` with restrictive filters).
  2. Bid cap / cost cap is set below market clearing price.
  3. All three diagnostic rankings (`quality_ranking`, `engagement_rate_ranking`, `conversion_rate_ranking`) are `BELOW_AVERAGE_*`.
- **Recommendation:** Broaden targeting (remove stacked interest constraints or test Advantage+ audience), relax restrictive bid caps, or introduce fresh creative formats suited to full-screen placements (9:16 Reels/Stories).

### Pattern B: Low Thumbstop Rate (`< 15%`) + Low Outbound CTR (`< 0.8%`)
- **Observed Signals:** Most impressions swipe past before 3 seconds (`actions[video_view] / impressions < 15%`), dragging down ThruPlay and Outbound CTR.
- **Root Cause:** **Opening Hook Failure.** The first 1–3 seconds (visual frame, motion pattern interrupt, or opening headline overlay) fail to stop the scroll or signal relevance to the target audience.
- **Recommendation:** Keep the core product demo body if unproven, but test 2–3 new first-3-second visual/audio hooks and opening text overlays. Check whether the video aspect ratio matches the dominant placement (9:16 for Reels/Stories).

### Pattern C: Healthy Thumbstop (`>= 20%`) + Weak Hold Rate (`< 12%`) / Steep `p25 -> p50` Drop
- **Observed Signals:** Viewers stop for the first 3 seconds, but average watch time is `<= 2–3s` and `video_p50_watched_actions` drops sharply relative to 3-second views.
- **Root Cause:** **Body Pacing or Hook-to-Body Disconnect.** The opening hook grabs attention, but the transition at seconds 3–8 is slow, generic, or fails to deliver immediately on the hook's promise.
- **Recommendation:** Tighten edits between seconds 3 and 10; front-load the strongest product demonstration beat ("show the payoff in action immediately after the hook") and cut filler setup.

### Pattern D: Fast Decision Clicks (Moderate Thumbstop + Short Watch Time + Strong Outbound CTR `>= 1.2%`)
- **Observed Signals:** Average watch time is short (`2–4s`) and ThruPlay rate is modest (`2%–5%`), **yet** Outbound CTR (`>= 1.2%–1.6%`), Cost per Outbound Click, and social engagement (likes/saves/shares) are healthy.
- **Root Cause:** **High-Intent Skim Behavior on Reels/Stories.** High-intent prospects recognize the problem/product in the opening frames + primary text/headline and click the CTA immediately without watching the full video, while non-prospects swipe past in `< 3s`.
- **Recommendation:** Do **not** misdiagnose this as a failing ad when Outbound CTR and CPC are strong. Preserve the winning ad, and test a shorter cut (`10–15s`) or a faster 2-second visual hook in a new iteration to lift thumbstop even higher.

### Pattern E: Good Video Hold / High All CTR (`> 2.0%`), but Low Outbound CTR (`< 0.8%`)
- **Observed Signals:** Users watch the video and tap the post (`clicks / impressions > 2%`, likes/saves present), but `outbound_clicks_ctr` is `< 0.8%` (`inline_link_clicks / clicks < 35%`).
- **Root Cause:** **Weak Offer Framing or CTA Disconnect.** The video entertains or informs like organic content, but gives no urgent, concrete reason to click through to the site right now.
- **Recommendation:** Test direct, action-oriented headlines and CTAs (e.g., comparing package/pricing framing vs generic app landing framing). In creative splits, concrete offer framing (such as viewing packages/pricing or starting a specific level) routinely doubles Outbound CTR over vague "learn more" copy using the exact same video.

### Pattern F: Sibling Ad Divergence Inside a CBO Ad Set
- **Observed Signals:** Two or more ads share the same ad set (`CBO` or `ABO`). After `>= 1,500` impressions each, Ad A has `~2x` the Outbound CTR and `~25–35%` lower Cost per Outbound Click than Ad B, and CBO begins routing `60%–80%+` of daily spend to Ad A.
- **Root Cause:** Clear creative/CTA/destination resonance winner. However, during the first 24–48 hours, CBO is still calibrating.
- **Recommendation:**
  - **If `< 48h` or total spend `< 1.5x` daily budget:** **Hold.** Let CBO naturally favor the stronger ad without resetting learning.
  - **At checkpoint (`>= 2,500–3,000` impressions on the lagging ad and `0` conversions while trailing by `>= 40%` in Link CTR):** Pause the lagging ad so 100% of budget consolidates into the winning variant, or replace the laggard in the next test batch.

### Pattern G: Placement Skew (Stories vs. Reels vs. Feed)
- **Observed Signals:** Placement breakdown (`publisher_platform,platform_position`) shows one placement (e.g., `instagram_stories`) driving higher Link CTR (`1.8%`) and lower Cost per Outbound Click (`IDR 1,000`), while another (`instagram_reels`) takes `70%` of spend at lower CPM (`IDR 14,500`) and drives most social proof (`likes`, `saves`, `shares`).
- **Root Cause:** Normal placement liquidity behavior—Reels supplies cheaper impression inventory and social engagement, while Stories often converts high-intent taps at a higher CTR.
- **Recommendation:** Keep both placements combined inside the ad set unless one placement consumes `> 30%` of spend over `>= 7` days with markedly worse CPA/Cost per Outbound Click and zero downstream value.

### Pattern H: Healthy Outbound Clicks, Zero or Low `landing_page_view` / Mid-Funnel Events
- **Observed Signals:** `outbound_clicks` is `>= 30–50+`, but `actions` contains no `landing_page_view` (`Connect Rate < 50%` or `0%`) and no `view_content` / `initiate_checkout` events.
- **Root Cause (Two Distinct Possibilities—Verify Which One Applies!):**
  1. **Intentional Privacy-Gated or Server-Side CAPI Architecture:** Some web apps intentionally restrict browser Meta Pixel `PageView` to static marketing roots (`/`) or require explicit ad-privacy consent before loading `fbevents.js`, capturing `fbclid` (`_fbc`/`_fbp`) and sending only server-side CAPI `Purchase` events after payment. In this architecture, `0` `landing_page_view` and `0` `initiate_checkout` in Meta Insights on `/app/*` routes is **expected by design**, not a broken site.
  2. **Unintentional Tracking Break or Mobile Load Drop-Off:** If the destination page is supposed to fire browser Pixel `PageView` unconditionally, a `< 70%` Outbound-to-LPV ratio means slow mobile page load (`> 3s`), redirect stripping `fbclid`, or broken Pixel initialization.
- **Recommendation:** Always check whether the target URL uses consent-gated/CAPI-only measurement. If CAPI-only, advise the user to cross-check first-party analytics (e.g., Firebase / backend checkout logs) for mid-funnel sign-ups and checkout starts alongside Meta's `Purchase` attribution. If browser Pixel was expected, audit page speed and Pixel firing on mobile.

### Pattern I: Strong Outbound Traffic & LPVs (`>= 150–300` Clicks), Zero or High-CPA Conversions
- **Observed Signals:** Outbound CTR (`>= 1.2%`) and CPC are healthy, visitors reach the landing page (`>= 150–300` outbound clicks and spend `>= 2x–3x` target CPA), but `Purchase` / `Lead` count is `0` or CPA exceeds target margin.
- **Root Cause:** **Post-Click Offer, Auth Wall, or Checkout Friction.** The ad succeeded at generating qualified curiosity, but the destination page failed to convert. Common culprits:
  - Sending cold ad traffic directly to a hard login wall or complex dashboard (`/app`) before showing value or pricing.
  - Mismatch between ad promise/headline and landing page headline.
  - Pricing friction, missing local payment methods, or multi-step checkout drop-off.
- **Recommendation:** Inspect first-party funnel drop-off (`Landing -> Sign-Up -> Package Selection -> Checkout Initiated -> Paid`). Test routing traffic to a transparent package/offer page or interactive preview, align landing page headline with the winning ad's copy, and reduce steps before checkout.

### Pattern J: Creative Fatigue & Audience Saturation (Established Campaigns `>= 7–14d`)
- **Observed Signals:** Comparing `last_7d` vs `previous-period` (`adsctl meta reports compare`) shows `Frequency` rising above `2.5–3.0`, First-Time Impression Ratio (`Reach / Impressions`) falling below `45%`, Outbound CTR compressing by `>= 20%`, and CPM/CPA climbing.
- **Root Cause:** The creative has saturated its responsive pocket within the target audience.
- **Recommendation:** Introduce 2–3 new creative angles (different visual format, persona, or hook) or expand audience constraints (e.g., test broader geo/interest/Advantage+ settings). If CPA has breached the maximum allowable threshold over the last 7 completed days, scale budget down by 15–20% or pause the fatigued ad once replacement creatives are active.

---

## 3. Turning Analyst Thresholds into `adsctl meta monitor` Rules

When the user wants ongoing automated checks for the thresholds identified in your audit, translate your checkpoints into a deterministic `adsctl meta monitor` YAML ruleset (and validate with `adsctl meta monitor validate --file rules.yaml`):

- Use `type: delivery-status` on `resource: ads` (`effective_status_in: [DISAPPROVED, WITH_ISSUES]`) to catch review/policy blocks immediately.
- Use `type: metric-threshold` with explicit `min_spend`, `min_actions`, `currency`, `attribution`, `action_report_time`, and `exclude_recent_days` so alerts only fire after statistically meaningful evidence (never on Day-1 noise).
