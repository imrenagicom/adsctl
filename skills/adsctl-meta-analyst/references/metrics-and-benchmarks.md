# Meta Ads Metrics, Formulas, and Benchmarks

Use this reference to map `adsctl meta insights query` and `adsctl meta reports` fields to derived analyst metrics, interpret action arrays accurately, and apply directional benchmarks.

---

## 1. Metric Definitions & Exact Formulas

Missing or unavailable fields in Meta Insights are **unavailable**, never zero. Do not sum `reach`, `frequency`, `unique_*`, or ratio metrics across rows, days, or placements.

### Stage 1: Delivery, Pacing & Saturation

| Metric | Meta / `adsctl` Source | Formula / Definition | Why Analysts Track It |
| :--- | :--- | :--- | :--- |
| **Spend Pacing** | `spend` vs `daily_budget` / `lifetime_budget` | $\frac{\text{Daily Spend}}{\text{Configured Daily Budget}} \times 100$ | Confirms whether the auction is clearing the target budget or throttling due to narrow targeting, bid caps, or delivery issues. |
| **CPM** | `cpm` (or $\frac{\text{spend}}{\text{impressions}} \times 1000$) | Cost per 1,000 impressions in account currency | Reflects audience competition, placement mix (Reels vs Stories vs Feed), and Meta's perceived quality/relevance of the ad. |
| **Frequency** | `frequency` (or $\frac{\text{impressions}}{\text{reach}}$) | Average impressions per unique reached user in the period | Detects audience saturation and creative fatigue when tracked over rolling 7-day and lifetime windows. |
| **First-Time Impression Ratio (FTIR)** | Derived from `reach` and `impressions` | $\frac{\text{Reach}}{\text{Impressions}} \times 100 = \frac{1}{\text{Frequency}} \times 100$ | Measures the share of impressions reaching fresh prospects vs recycling the same pool. |
| **Diagnostic Rankings** | `quality_ranking`, `engagement_rate_ranking`, `conversion_rate_ranking` | `ABOVE_AVERAGE`, `AVERAGE`, `BELOW_AVERAGE_35`, `BELOW_AVERAGE_20`, `BELOW_AVERAGE_10`, `UNKNOWN` | Compares the ad against competing ads targeting the same audience. Returns `UNKNOWN` until the ad accumulates sufficient impressions (~500–1,000+). |

### Stage 2: Creative Hook, Retention & Engagement (Video & Static)

| Metric | Meta / `adsctl` Source | Formula / Definition | Why Analysts Track It |
| :--- | :--- | :--- | :--- |
| **Thumbstop Rate (3s Hook Rate)** | `actions[action_type="video_view"]` and `impressions` | $\frac{\text{3-Second Video Views}}{\text{Impressions}} \times 100$ | Measures scroll-stopping power in the first 3 seconds (visual opener, headline overlay, pattern interrupt). |
| **ThruPlay Rate** | `video_thruplay_watched_actions` and `impressions` | $\frac{\text{ThruPlays (15s or completion)}}{\text{Impressions}} \times 100$ | Measures meaningful video consumption relative to everyone who saw the impression. |
| **Hold Rate (Hook-to-ThruPlay)** | `video_thruplay_watched_actions` and `actions[video_view]` | $\frac{\text{ThruPlays}}{\text{3-Second Video Views}} \times 100$ | Isolates whether viewers who stopped for the hook stayed for the core pitch. |
| **Video Retention Curve** | `video_p25_watched_actions`, `video_p50_watched_actions`, `video_p75_watched_actions`, `video_p95_watched_actions`, `video_p100_watched_actions` | $\frac{\text{Video Watched at } X\%}{\text{3-Second Video Views (or Impressions)}} \times 100$ | Pinpoints the exact quartile where viewers drop off (e.g., slow demo transition between 25% and 50%). |
| **Average Watch Time** | `video_avg_time_watched_actions` | Seconds watched per play | Indicates whether users consume the narrative or make a snap 2–3 second decision. |
| **Social Proof Velocity** | `actions`: `post_reaction`, `onsite_conversion.post_save`, `post` (shares), `comment` | Reactions, saves, and shares per 1,000 impressions | High save/share rates indicate high-intent resonance and lower auction penalties over time. |

### Stage 3: Click Efficiency & Outbound Intent

| Metric | Meta / `adsctl` Source | Formula / Definition | Why Analysts Track It |
| :--- | :--- | :--- | :--- |
| **CTR (All Clicks)** | `ctr` (`clicks` / `impressions`) | $\frac{\text{All Clicks}}{\text{Impressions}} \times 100$ | Includes link clicks **plus** profile taps, "See more" text expansions, reactions, comments, and video unmute/fullscreen taps. |
| **Inline Link CTR** | `inline_link_click_ctr` (`inline_link_clicks` / `impressions`) | $\frac{\text{Inline Link Clicks}}{\text{Impressions}} \times 100$ | Measures clicks on ad links/CTA buttons leading to a destination experience. |
| **Outbound CTR** | `outbound_clicks_ctr` (`outbound_clicks` / `impressions`) | $\frac{\text{Outbound Clicks}}{\text{Impressions}} \times 100$ | Strictly measures clicks taking users **off** Meta properties to your external website or app store. Primary traffic-intent KPI for external websites. |
| **Unique Link CTR** | `unique_inline_link_click_ctr` | $\frac{\text{Unique Inline Link Clicks}}{\text{Reach}} \times 100$ | Eliminates repeat clicks from the same user to show true per-person click propensity. |
| **Link-to-All Click Ratio** | `inline_link_clicks` / `clicks` | $\frac{\text{Inline Link Clicks}}{\text{All Clicks}} \times 100$ | Shows whether users are clicking the CTA (`> 50%`) or mostly tapping the profile/reading copy/reacting (`< 35%`). |
| **Cost per Outbound Click (Link CPC)** | `cost_per_outbound_click` / `cost_per_inline_link_click` | $\frac{\text{Spend}}{\text{Outbound (or Link) Clicks}} = \frac{\text{CPM}}{\text{Outbound CTR} \times 10}$ | Governed mathematically by CPM and Outbound CTR; isolates the unit cost of acquiring a site visitor. |

### Stage 4: Post-Click Bridge & Mid-Funnel Continuity

| Metric | Meta / `adsctl` Source | Formula / Definition | Why Analysts Track It |
| :--- | :--- | :--- | :--- |
| **Landing Page Views (LPV)** | `actions[action_type="landing_page_view"]` or `omni_landing_page_view` | Count of destination page loads where the browser Meta Pixel fired `PageView` | Confirms that clicked traffic actually loaded the destination page with an active browser pixel. |
| **Outbound Click-to-LPV Ratio (Connect Rate)** | `landing_page_view` and `outbound_clicks` | $\frac{\text{Landing Page Views}}{\text{Outbound Clicks}} \times 100$ | Detects mobile load latency, redirect stripping, consent gates, or intentionally omitted browser pixels. |
| **Cost per Landing Page View** | `cost_per_action_type[landing_page_view]` | $\frac{\text{Spend}}{\text{Landing Page Views}}$ | True cost per arrived, tracked visitor. |
| **Mid-Funnel Progression** | `actions`: `view_content`, `add_to_cart`, `initiate_checkout`, `lead`, `complete_registration` | Action counts and step-to-step conversion rates | Separates product/pricing interest (`view_content`, `initiate_checkout`) from payment/checkout completion (`purchase`). |

### Stage 5: Conversion Economics & Bottom-Line Return

| Metric | Meta / `adsctl` Source | Formula / Definition | Why Analysts Track It |
| :--- | :--- | :--- | :--- |
| **Conversions (Target Action)** | `actions[action_type="<target_event>"]` | Count of optimization objective events (e.g., `purchase` / `offsite_conversion.fb_pixel_purchase`) | Primary volume metric for `OUTCOME_SALES` and `OUTCOME_LEADS` campaigns, and the input to Meta's 50-events/week learning threshold. |
| **Click-to-Conversion Rate (CVR)** | Target conversions and `outbound_clicks` (or `landing_page_view`) | $\frac{\text{Conversions}}{\text{Outbound Clicks (or LPV)}} \times 100$ | Measures post-click landing page, offer, pricing, and checkout persuasion. |
| **CPA (Cost per Action)** | `cost_per_action_type` or `adsctl meta reports rank --metric cpa --action-type ...` | $\frac{\text{Spend}}{\text{Target Conversions}}$ | Primary efficiency KPI; must remain below target unit-economic margin. |
| **ROAS (Return on Ad Spend)** | `purchase_roas` or $\frac{\text{action\_values[event]}}{\text{Spend}}$ | $\frac{\text{Attributed Conversion Value}}{\text{Spend}}$ | Measures gross revenue generated per unit of ad spend in the account's currency. |
| **Average Order Value (AOV)** | `action_values[event]` and `actions[event]` | $\frac{\text{Attributed Conversion Value}}{\text{Target Conversions}}$ | Explains why a campaign with healthy CPA may still have low ROAS (e.g., buyers choosing the lowest-tier package). |

---

## 2. Common Meta `actions` and `action_values` Keys

In `adsctl meta insights query`, the `actions`, `action_values`, and `cost_per_action_type` arrays contain entries of `{ "action_type": "<key>", "value": "<number>" }`. Note that Meta often emits both canonical and omni/pixel-prefixed keys for the same underlying event—**do not sum duplicate aliases for the same event**:

- **Clicks & Traffic:**
  - `link_click`: Inline link clicks
  - `outbound_click` (also in `outbound_clicks` field): Clicks leaving Meta properties
  - `landing_page_view` / `omni_landing_page_view`: Browser Pixel `PageView` on the destination URL
- **Video & Post Engagement:**
  - `video_view`: 3-second continuous video views
  - `post_engagement` / `page_engagement`: Total post/page interactions
  - `post_reaction` / `onsite_conversion.post_net_like`: Reactions / likes
  - `onsite_conversion.post_save` / `onsite_conversion.post_net_save`: Post saves / bookmarks
  - `post`: Post shares
  - `comment`: Post comments
- **Website & App Conversions (check which alias the account's Pixel/CAPI uses; inspect without double-counting):**
  - **View Content:** `view_content`, `omni_view_content`, `offsite_conversion.fb_pixel_view_content`
  - **Add to Cart:** `add_to_cart`, `omni_add_to_cart`, `offsite_conversion.fb_pixel_add_to_cart`
  - **Initiate Checkout:** `initiate_checkout`, `omni_initiated_checkout`, `offsite_conversion.fb_pixel_initiate_checkout`
  - **Lead:** `lead`, `onsite_conversion.lead_grouped`, `offsite_conversion.fb_pixel_lead`
  - **Complete Registration:** `complete_registration`, `omni_complete_registration`, `offsite_conversion.fb_pixel_complete_registration`
  - **Purchase:** `purchase`, `omni_purchase`, `offsite_conversion.fb_pixel_purchase`
  - **Custom Events:** `offsite_conversion.fb_pixel_custom.<EventName>` or `<CustomEventName>`

When running `adsctl meta reports rank` or `adsctl meta monitor check` with `metric: cpa` or `metric: roas`, pass the exact `action_type` key present in the account's `actions` / `action_values` array.

---

## 3. Directional Benchmarks by Funnel Stage

Benchmarks vary by geography, currency, vertical, price point, and placement mix (e.g., Indonesia/SEA consumer apps vs US B2B SaaS have very different absolute CPMs and CPCs, while rate metrics like Thumbstop, Hold, Outbound CTR, and Connect Rate are more comparable across markets). Always prioritize **intra-account historical baselines and sibling-ad comparisons** first, then use these directional reference bands:

| Metric | Strong / Healthy | Watch / Average | Weak / Bottleneck | Context & Caveats |
| :--- | :--- | :--- | :--- | :--- |
| **Frequency (7-Day Cold Prospecting)** | `1.0 – 1.8` | `1.9 – 2.7` | `> 2.8 – 3.0+` | Retargeting pools can sustain `3.0 – 5.0`, but cold prospecting above `2.5–3.0` with rising CPM/falling CTR signals creative saturation. |
| **First-Time Impression Ratio (Cold)** | `> 60%` | `45% – 60%` | `< 45%` | If `< 45%` on prospecting, the audience is too narrow or the creative angle has capped out its addressable pool. |
| **Thumbstop Rate (3s Video Views / Impr.)** | `>= 25% – 30%+` | `15% – 24%` | `< 15%` | Reels/Stories fast-scroll environments often see `12%–25%` on product demos; `< 12%` means the opening visual/text hook is being swiped past immediately. |
| **Hold Rate (ThruPlay / 3s Views)** | `>= 20% – 30%+` | `12% – 19%` | `< 12%` | High thumbstop + low hold means the opening hook got attention, but the middle demo/script lost the viewer. Note: very short videos (`<= 15s`) naturally have higher ThruPlay ratios than `45s+` explainers. |
| **Outbound / Inline Link CTR** | `>= 1.5% – 2.5%+` | `0.9% – 1.49%` | `< 0.8%` | For conversion-optimized (`OUTCOME_SALES`) cold campaigns, `1.0%–1.5%` is solid and `> 1.5%` is strong. Traffic-optimized campaigns naturally show higher CTR but much lower conversion intent. |
| **All CTR vs Outbound CTR Gap** | Outbound is `>= 50%` of All CTR | Outbound is `35% – 49%` of All CTR | Outbound is `< 30%` of All CTR | If All CTR is high (`> 2.5%`) while Outbound CTR is low (`< 0.6%`), users are engaging with the post/video/profile without clicking the CTA link. |
| **Outbound Click-to-LPV Connect Rate** | `>= 80% – 90%` | `65% – 79%` | `< 65%` (or `0%` when browser Pixel is expected) | If `< 65%` when browser Pixel is active on the landing page, investigate mobile load time, redirects, or script blockers. **Important exception:** Privacy-gated or CAPI-only landing pages intentionally omit browser `PageView` on app routes; verify site tracking design before diagnosing a broken site. |
| **Landing Page / Click-to-Purchase CVR** | `>= 1.5% – 3.5%+` | `0.8% – 1.4%` | `< 0.7%` (after `>= 150–300` clicks) | Depends heavily on price point (AOV) and checkout friction (e.g., instant QRIS/e-wallet vs multi-step account creation + paywall). |

---

## 4. Objective-Specific Primary & Secondary KPIs

Always evaluate a campaign against its configured `objective` and ad set `optimization_goal`:

1. **`OUTCOME_SALES` / `OFFSITE_CONVERSIONS` (Purchase / InitiateCheckout / AddToCart):**
   - **Primary KPIs (Mature):** CPA (`cost_per_action_type[purchase]`), ROAS (`purchase_roas`), Purchase Volume, Click-to-Purchase CVR.
   - **Primary KPIs (Launch / `< 48h` / Pre-Conversion):** Outbound CTR, Cost per Outbound Click, Thumbstop & Hold Rate, Mid-Funnel/LPV continuity, Sibling Ad CTR separation.
2. **`OUTCOME_LEADS` / `OFFSITE_CONVERSIONS` or `LEAD_GENERATION`:**
   - **Primary KPIs:** Cost per Lead (CPL), Lead CVR (`leads / outbound_clicks`), Downstream Lead Qualification Rate.
   - **Secondary KPIs:** Outbound CTR, Connect Rate, Form Completion Rate.
3. **`OUTCOME_TRAFFIC` / `LINK_CLICKS` or `LANDING_PAGE_VIEWS`:**
   - **Primary KPIs:** Cost per Landing Page View, Outbound Click-to-LPV Connect Rate (`>= 80%`), Outbound CTR, Cost per Outbound Click.
   - **Analyst Warning:** `LINK_CLICKS` optimization buys cheap clickers who rarely purchase. If the business goal is sales or sign-ups, recommend testing `OFFSITE_CONVERSIONS` once conversion tracking is verified.
4. **`OUTCOME_ENGAGEMENT` / `THRUPLAY` or `POST_ENGAGEMENT`:**
   - **Primary KPIs:** Cost per ThruPlay, ThruPlay Rate, Hold Rate (`p25 -> p100`), Social Proof Velocity (saves/shares/reactions per 1k impressions).
5. **`OUTCOME_APP_PROMOTION` / `APP_INSTALLS` or `OFFSITE_CONVERSIONS` (App Events):**
   - **Primary KPIs:** Cost per Install (CPI), Outbound Click-to-Install CVR, Cost per Trial/Subscription/In-App Purchase.

---

## 5. Attribution, Reporting Time, and Timezone Rules

- **Attribution Mode (`--attribution`):**
  - `adset` (default in `adsctl`, sets `use_unified_attribution_setting=true`): Uses each ad set's optimization attribution window (commonly `7d_click, 1d_view` or `7d_click`). Best for matching Meta Ads Manager's default column and understanding how the delivery algorithm grades each ad set.
  - `windows` (`--attribution windows --attribution-windows 7d_click,1d_view`): Splits or constrains actions to explicit windows. Comparing `7d_click` against `1d_view` reveals whether reported conversions are click-driven or inflated by view-through credit.
- **Action Report Time (`--action-report-time`):**
  - `impression` (default): Credits conversions back to the date the ad impression occurred. Recent days (`yesterday` / `today`) can gain delayed conversions over the attribution window.
  - `conversion`: Credits conversions to the date the conversion occurred, matching backend/first-party daily revenue logs more closely.
- **Completed vs Partial Days:**
  - Always check the account's `timezone_name` (e.g., `Asia/Jakarta`).
  - `adsctl meta reports` (`summary`, `compare`, `rank`, `breakdown`) resolve `last_Nd` to **completed account-local days** (`today - N` through `yesterday`). Pass `--include-partial-period` only when intentionally including today.
  - `adsctl meta insights query --date-preset maximum` includes today's partial impressions and spend; always run `--time-increment 1` alongside it so you can separate completed-day stability from today's early hours.
