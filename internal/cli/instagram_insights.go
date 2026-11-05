package cli

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"github.com/spf13/cobra"
)

// Instagram insights read organic performance through the Instagram Graph API
// (Facebook Login path). The token needs instagram_basic,
// instagram_manage_insights and pages_read_engagement for the IG user.
//
// Media insights reject a whole request when one metric does not apply to the
// media's product type, so defaults are chosen per FEED, REELS and STORY.
// Values are Meta's: unavailable data is omitted rather than reported as zero.

const (
	igFeedMetrics    = "reach,views,likes,comments,shares,saved,total_interactions,follows,profile_visits"
	igReelMetrics    = "reach,views,likes,comments,shares,saved,total_interactions,ig_reels_avg_watch_time,ig_reels_video_view_total_time"
	igStoryMetrics   = "reach,views,replies,shares,total_interactions,follows,profile_visits,navigation"
	igAccountMetrics = "reach,views,accounts_engaged,total_interactions,likes,comments,shares,saves,replies,profile_links_taps"
	igInsightFields  = "id,media_type,media_product_type,permalink,timestamp"
	igMaxInsightIDs  = 50
	igTimestamp      = "2006-01-02T15:04:05-0700"
)

var igPresets = map[string]int{"today": 0, "yesterday": 1, "last_7d": 7, "last_14d": 14, "last_28d": 28, "last_30d": 30}

func igDefaultMetrics(product string) string {
	switch product {
	case "FEED":
		return igFeedMetrics
	case "REELS":
		return igReelMetrics
	case "STORY":
		return igStoryMetrics
	}
	return ""
}

func (a *App) addInstagramInsights(ig *cobra.Command) {
	g := group("insights", "Read organic performance for Instagram media and accounts")
	ig.AddCommand(g)
	a.addIGMediaInsights(g)
	a.addIGAccountInsights(g)
	a.addIGAudienceInsights(g)
}

func (a *App) addIGMediaInsights(g *cobra.Command) {
	var user, metrics, breakdown, types, since, until string
	var stories, raw bool
	var maxItems int
	cmd := action("media [ID...]", "Lifetime metrics for posts, reels and stories (by ID or the IG user's recent media)", "read", func(c *cobra.Command, args []string) error {
		if len(args) > igMaxInsightIDs {
			return output.Fail("USAGE", fmt.Sprintf("At most %d media IDs per request.", igMaxInsightIDs), 2)
		}
		for _, id := range args {
			if err := config.ID(id); err != nil {
				return err
			}
		}
		listing := len(args) == 0
		if !listing {
			for _, n := range []string{"stories", "type", "since", "until", "max-items"} {
				if c.Flags().Changed(n) {
					return output.Fail("USAGE", "--"+n+" applies only when listing an IG user's media (no IDs).", 2)
				}
			}
		}
		if maxItems < 1 || maxItems > 500 {
			return output.Fail("USAGE", "--max-items must be 1..500.", 2)
		}
		want, err := igTypes(types)
		if err != nil {
			return err
		}
		from, to, err := igRange(since, until, "")
		if err != nil {
			return err
		}
		r, err := a.igRuntime(c, &user, listing, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		var media []map[string]any
		if listing {
			path := user + "/media"
			if stories {
				path = user + "/stories"
			}
			q := url.Values{"fields": {igInsightFields}}
			// The media edge pages by time; results are also filtered locally.
			if !from.IsZero() && !stories {
				q.Set("since", strconv.FormatInt(from.Unix(), 10))
			}
			if !to.IsZero() && !stories {
				q.Set("until", strconv.FormatInt(to.Unix(), 10))
			}
			res, e := r.Client.List(r.Ctx, path, q, graph.PageOptions{Scope: r.Profile + "|" + user, All: true, PageSize: min(maxItems, 100), MaxItems: maxItems})
			if e != nil {
				return e
			}
			a.meta["media_complete"] = res.Complete
			for _, row := range res.Rows {
				m, _ := row.(map[string]any)
				if m == nil || !igInRange(m, from, to) || (len(want) > 0 && !slices.Contains(want, fmt.Sprint(m["media_product_type"]))) {
					continue
				}
				media = append(media, m)
			}
		} else {
			for _, id := range args {
				v, e := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {igInsightFields}})
				if e != nil {
					if len(args) == 1 {
						return e
					}
					media = append(media, map[string]any{"id": id, "error": igRowError(e)})
					continue
				}
				media = append(media, v.Value)
			}
		}
		a.meta["metric_selection"] = "per media product type"
		if metrics != "" {
			a.meta["metric_selection"] = "explicit"
		}
		rows := []any{}
		var warnings []output.Warning
		failed := 0
		for _, m := range media {
			row := map[string]any{}
			for k, v := range m {
				row[k] = v
			}
			if _, bad := row["error"]; bad {
				failed++
				rows = append(rows, row)
				continue
			}
			product := fmt.Sprint(m["media_product_type"])
			set := metrics
			if set == "" {
				set = igDefaultMetrics(product)
			}
			if set == "" {
				row["error"] = map[string]any{"code": "UNSUPPORTED_MEDIA", "message": "No default metrics for media_product_type " + product + "; pass --metric."}
				failed++
				rows = append(rows, row)
				continue
			}
			q := url.Values{"metric": {set}}
			if breakdown != "" {
				q.Set("breakdown", breakdown)
			}
			v, e := r.Client.Request(r.Ctx, "GET", fmt.Sprint(m["id"])+"/insights", q)
			if e != nil {
				if len(media) == 1 || igFatal(e) {
					rows = append(rows, row)
					return a.partialRows(c, rows, e)
				}
				row["error"] = igRowError(e)
				failed++
				rows = append(rows, row)
				continue
			}
			data, _ := v.Value["data"].([]any)
			igFlatten(data, row)
			if raw {
				row["insights"] = data
			}
			rows = append(rows, row)
		}
		if failed > 0 {
			warnings = append(warnings, output.Warning{Code: "PARTIAL_INSIGHTS", Message: fmt.Sprintf("%d of %d media returned no insights; see each row's error.", failed, len(rows))})
		}
		if listing && len(rows) == 0 {
			warnings = append(warnings, output.Warning{Code: "NO_MEDIA", Message: "No media matched the selection; stories are listed only with --stories and only while live (24 hours)."})
		}
		if failed > 0 && failed == len(rows) {
			return a.partialRows(c, rows, output.Fail("INSIGHTS_UNAVAILABLE", "No selected media returned insights.", 8))
		}
		return a.emitWith(c, rows, warnings)
	})
	cmd.Args = cobra.ArbitraryArgs
	f := cmd.Flags()
	f.StringVar(&user, "ig-user", "", "Instagram user whose recent media to read (or ADSCTL_IG_USER); not needed with IDs")
	f.StringVar(&metrics, "metric", "", "Native metrics, comma-separated (default: chosen per FEED, REELS or STORY)")
	f.StringVar(&breakdown, "breakdown", "", "action_type (profile_activity) or story_navigation_action_type (navigation)")
	f.StringVar(&types, "type", "", "Keep only these product types: feed, reels, story (comma-separated)")
	f.StringVar(&since, "since", "", "Keep media published on or after this date (YYYY-MM-DD UTC, RFC 3339 or Unix seconds)")
	f.StringVar(&until, "until", "", "Keep media published on or before this date (inclusive)")
	f.BoolVar(&stories, "stories", false, "Read the IG user's live stories instead of feed and reels")
	f.BoolVar(&raw, "raw", false, "Include Meta's native insights rows under insights")
	f.IntVar(&maxItems, "max-items", 25, "Most recent media to read, 1..500")
	g.AddCommand(cmd)
}

func (a *App) addIGAccountInsights(g *cobra.Command) {
	var user, metrics, metricType, period, breakdown, since, until, preset string
	var raw bool
	cmd := action("account", "Account reach, views and interactions over a reporting period", "read", func(c *cobra.Command, _ []string) error {
		if metricType != "total_value" && metricType != "time_series" {
			return output.Fail("USAGE", "--metric-type must be total_value or time_series.", 2)
		}
		if breakdown != "" && metricType != "total_value" {
			return output.Fail("USAGE", "Breakdowns require --metric-type total_value.", 2)
		}
		if c.Flags().Changed("since") || c.Flags().Changed("until") {
			if c.Flags().Changed("date-preset") {
				return output.Fail("USAGE", "Choose --date-preset or --since/--until, not both.", 2)
			}
			if since == "" || until == "" {
				return output.Fail("USAGE", "Account insights need both --since and --until.", 2)
			}
			preset = ""
		}
		from, to, err := igRange(since, until, preset)
		if err != nil {
			return err
		}
		if !to.After(from) {
			return output.Fail("USAGE", "--until must not be before --since.", 2)
		}
		r, err := a.igRuntime(c, &user, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		q := url.Values{"metric": {metrics}, "period": {period}, "metric_type": {metricType}, "since": {strconv.FormatInt(from.Unix(), 10)}, "until": {strconv.FormatInt(to.Unix(), 10)}}
		if breakdown != "" {
			q.Set("breakdown", breakdown)
		}
		a.meta["request"] = q
		a.meta["range"] = map[string]any{"since": from.Format(time.RFC3339), "until": to.Format(time.RFC3339), "timezone": "UTC", "until_exclusive": true}
		v, err := r.Client.Request(r.Ctx, "GET", user+"/insights", q)
		if err != nil {
			return err
		}
		data, _ := v.Value["data"].([]any)
		var rows []any
		if metricType == "time_series" {
			rows = igSeries(data)
		} else {
			row := map[string]any{"since": from.Format(time.RFC3339), "until": to.Format(time.RFC3339)}
			igFlatten(data, row)
			if raw {
				row["insights"] = data
			}
			rows = []any{row}
		}
		warnings := []output.Warning{{Code: "DATA_LAG", Message: "Instagram account insights can lag up to 48 hours; missing metrics mean no data, not zero."}}
		if len(data) == 0 {
			warnings = append(warnings, output.Warning{Code: "NO_ROWS", Message: "Meta returned no insights for this range and metric set."})
		}
		return a.emitWith(c, rows, warnings)
	})
	f := cmd.Flags()
	f.StringVar(&user, "ig-user", "", "Instagram user ID (or ADSCTL_IG_USER)")
	f.StringVar(&metrics, "metric", igAccountMetrics, "Native account metrics, comma-separated")
	f.StringVar(&metricType, "metric-type", "total_value", "total_value (one total) or time_series (per day; reach only)")
	f.StringVar(&period, "period", "day", "Native period")
	f.StringVar(&breakdown, "breakdown", "", "media_product_type, follow_type, follower_type or contact_button_type (total_value only)")
	f.StringVar(&preset, "date-preset", "last_7d", "today, yesterday, last_7d, last_14d, last_28d or last_30d (complete UTC days)")
	f.StringVar(&since, "since", "", "Inclusive start (YYYY-MM-DD UTC, RFC 3339 or Unix seconds)")
	f.StringVar(&until, "until", "", "Inclusive end date")
	f.BoolVar(&raw, "raw", false, "Include Meta's native insights rows under insights")
	g.AddCommand(cmd)
}

func (a *App) addIGAudienceInsights(g *cobra.Command) {
	var user, audience, breakdown, timeframe string
	cmd := action("audience", "Follower or engaged-audience demographics by age, city, country or gender", "read", func(c *cobra.Command, _ []string) error {
		metric := map[string]string{"followers": "follower_demographics", "engaged": "engaged_audience_demographics"}[audience]
		if metric == "" {
			return output.Fail("USAGE", "--audience must be followers or engaged.", 2)
		}
		dims := strings.Split(breakdown, ",")
		for _, d := range dims {
			if !slices.Contains([]string{"age", "city", "country", "gender"}, d) {
				return output.Fail("USAGE", "--breakdown must be age, city, country or gender (comma-separated).", 2)
			}
		}
		if timeframe != "this_week" && timeframe != "this_month" {
			return output.Fail("USAGE", "--timeframe must be this_week or this_month.", 2)
		}
		r, err := a.igRuntime(c, &user, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		q := url.Values{"metric": {metric}, "period": {"lifetime"}, "metric_type": {"total_value"}, "breakdown": {breakdown}, "timeframe": {timeframe}}
		a.meta["request"] = q
		v, err := r.Client.Request(r.Ctx, "GET", user+"/insights", q)
		if err != nil {
			return err
		}
		data, _ := v.Value["data"].([]any)
		rows := igDemographics(data)
		var warnings []output.Warning
		if len(rows) == 0 {
			warnings = append(warnings, output.Warning{Code: "NO_ROWS", Message: "Meta returned no demographics; they need at least 100 followers (or 100 engagements in the timeframe for engaged)."})
		} else {
			warnings = append(warnings, output.Warning{Code: "TOP_RESULTS", Message: "Meta returns only the top 45 values and counts only viewers with demographic data; totals can be below the follower count."})
		}
		return a.emitWith(c, rows, warnings)
	})
	f := cmd.Flags()
	f.StringVar(&user, "ig-user", "", "Instagram user ID (or ADSCTL_IG_USER)")
	f.StringVar(&audience, "audience", "followers", "followers or engaged")
	f.StringVar(&breakdown, "breakdown", "age,gender", "age, city, country or gender (comma-separated)")
	f.StringVar(&timeframe, "timeframe", "this_month", "this_week or this_month")
	g.AddCommand(cmd)
}

// emitWith writes a successful envelope with explicit warnings.
func (a *App) emitWith(c *cobra.Command, data any, warnings []output.Warning) error {
	meta := map[string]any{"command": c.CommandPath(), "complete": true}
	for k, v := range a.meta {
		meta[k] = v
	}
	env := output.New(data, meta)
	env.Warnings = append(env.Warnings, warnings...)
	return output.Write(a.Out, a.opts.Output, env)
}

// partialRows writes the rows gathered so far with the error, then returns it.
func (a *App) partialRows(c *cobra.Command, rows []any, err error) error {
	meta := map[string]any{"command": c.CommandPath(), "complete": false}
	for k, v := range a.meta {
		meta[k] = v
	}
	meta["complete"] = false
	env := output.New(rows, meta)
	env.OK = false
	env.Error = output.AsError(err)
	_ = output.Write(a.Out, a.opts.Output, env)
	return err
}

// igFatal reports errors that would fail every remaining media request too.
func igFatal(err error) bool {
	e := output.AsError(err)
	return e.Exit == 3 || e.Exit == 6 || e.Exit == 7 || (e.Exit == 4 && !strings.Contains(strings.ToLower(e.Message), "not enough viewers"))
}

func igRowError(err error) map[string]any {
	e := output.AsError(err)
	return map[string]any{"code": e.Code, "message": e.Message}
}

// igFlatten copies each metric's value into row by name. Breakdown results
// become "<metric>_by_<dimension>" maps keyed by the dimension values.
func igFlatten(data []any, row map[string]any) {
	for _, x := range data {
		m, _ := x.(map[string]any)
		name, _ := m["name"].(string)
		if name == "" {
			continue
		}
		if tv, ok := m["total_value"].(map[string]any); ok {
			if v, ok := tv["value"]; ok {
				row[name] = v
			}
			breakdowns, _ := tv["breakdowns"].([]any)
			for _, b := range breakdowns {
				bm, _ := b.(map[string]any)
				keys := igStrings(bm["dimension_keys"])
				results, _ := bm["results"].([]any)
				out := map[string]any{}
				for _, res := range results {
					rm, _ := res.(map[string]any)
					out[strings.Join(igStrings(rm["dimension_values"]), "|")] = rm["value"]
				}
				row[name+"_by_"+strings.Join(keys, "_")] = out
			}
			continue
		}
		if values, ok := m["values"].([]any); ok && len(values) == 1 {
			if vm, ok := values[0].(map[string]any); ok {
				row[name] = vm["value"]
			}
		}
	}
}

// igSeries turns time_series metrics into one row per end_time.
func igSeries(data []any) []any {
	byTime := map[string]map[string]any{}
	for _, x := range data {
		m, _ := x.(map[string]any)
		name, _ := m["name"].(string)
		values, _ := m["values"].([]any)
		for _, v := range values {
			vm, _ := v.(map[string]any)
			end := fmt.Sprint(vm["end_time"])
			if byTime[end] == nil {
				byTime[end] = map[string]any{"end_time": vm["end_time"]}
			}
			byTime[end][name] = vm["value"]
		}
	}
	keys := make([]string, 0, len(byTime))
	for k := range byTime {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]any, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, byTime[k])
	}
	return rows
}

// igDemographics returns one row per dimension combination, largest first.
func igDemographics(data []any) []any {
	rows := []any{}
	for _, x := range data {
		m, _ := x.(map[string]any)
		tv, _ := m["total_value"].(map[string]any)
		breakdowns, _ := tv["breakdowns"].([]any)
		for _, b := range breakdowns {
			bm, _ := b.(map[string]any)
			keys := igStrings(bm["dimension_keys"])
			results, _ := bm["results"].([]any)
			for _, res := range results {
				rm, _ := res.(map[string]any)
				row := map[string]any{"value": rm["value"]}
				for i, v := range igStrings(rm["dimension_values"]) {
					if i < len(keys) {
						row[keys[i]] = v
					}
				}
				rows = append(rows, row)
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return igNumber(rows[i].(map[string]any)["value"]) > igNumber(rows[j].(map[string]any)["value"])
	})
	return rows
}

func igStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, x := range items {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func igNumber(v any) float64 {
	f, _ := strconv.ParseFloat(fmt.Sprint(v), 64)
	return f
}

func igTypes(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var out []string
	for _, t := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "feed", "post", "posts":
			out = append(out, "FEED")
		case "reel", "reels":
			out = append(out, "REELS")
		case "story", "stories":
			out = append(out, "STORY")
		default:
			return nil, output.Fail("USAGE", "--type must be feed, reels or story (comma-separated).", 2)
		}
	}
	return out, nil
}

// igRange resolves --since/--until or a preset into [from, to) in UTC. Dates
// are whole UTC days and until is inclusive, so it maps to the next midnight.
func igRange(since, until, preset string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error
	if preset != "" {
		days, ok := igPresets[preset]
		if !ok {
			return from, to, output.Fail("USAGE", "--date-preset must be today, yesterday, last_7d, last_14d, last_28d or last_30d.", 2)
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		switch preset {
		case "today":
			return today, time.Now().UTC().Truncate(time.Second), nil
		case "yesterday":
			return today.AddDate(0, 0, -1), today, nil
		}
		return today.AddDate(0, 0, -days), today, nil
	}
	if since != "" {
		if from, err = igTime(since, false); err != nil {
			return from, to, err
		}
	}
	if until != "" {
		if to, err = igTime(until, true); err != nil {
			return from, to, err
		}
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return from, to, output.Fail("USAGE", "--until must not be before --since.", 2)
	}
	return from, to, nil
}

func igTime(s string, end bool) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if end {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0).UTC(), nil
	}
	return time.Time{}, output.Fail("USAGE", "Dates must be YYYY-MM-DD, RFC 3339 or Unix seconds.", 2)
}

func igInRange(m map[string]any, from, to time.Time) bool {
	if from.IsZero() && to.IsZero() {
		return true
	}
	t, err := time.Parse(igTimestamp, fmt.Sprint(m["timestamp"]))
	if err != nil {
		return false
	}
	return (from.IsZero() || !t.Before(from)) && (to.IsZero() || t.Before(to))
}
