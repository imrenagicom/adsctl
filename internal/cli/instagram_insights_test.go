package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"adsctl/internal/output"
)

func igRows(t *testing.T, out *bytes.Buffer) (output.Envelope, []map[string]any) {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err, out.String())
	}
	items, _ := env.Data.([]any)
	rows := make([]map[string]any, 0, len(items))
	for _, x := range items {
		m, _ := x.(map[string]any)
		rows = append(rows, m)
	}
	return env, rows
}

func TestInstagramMediaInsightsChooseMetricsPerType(t *testing.T) {
	want := map[string]string{"/v25.0/11/insights": igReelMetrics, "/v25.0/12/insights": igFeedMetrics}
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/1784/media":
			if r.URL.Query().Get("fields") != igInsightFields || r.URL.Query().Get("since") != "1761955200" {
				t.Errorf("media query: %v", r.URL.Query())
			}
			_, _ = fmt.Fprint(w, `{"data":[
				{"id":"11","media_type":"VIDEO","media_product_type":"REELS","timestamp":"2025-11-03T10:00:00+0000"},
				{"id":"12","media_type":"CAROUSEL_ALBUM","media_product_type":"FEED","timestamp":"2025-11-02T10:00:00+0000"},
				{"id":"13","media_type":"VIDEO","media_product_type":"AD","timestamp":"2025-11-02T09:00:00+0000"},
				{"id":"14","media_type":"IMAGE","media_product_type":"FEED","timestamp":"2025-10-30T10:00:00+0000"}]}`)
		case "/v25.0/11/insights", "/v25.0/12/insights":
			if got := r.URL.Query().Get("metric"); got != want[r.URL.Path] {
				t.Errorf("%s metric = %s", r.URL.Path, got)
			}
			_, _ = fmt.Fprint(w, `{"data":[{"name":"reach","period":"lifetime","values":[{"value":120}]},{"name":"views","period":"lifetime","values":[{"value":340}]}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "insights", "media", "--profile", "test", "--ig-user", "1784", "--since", "2025-11-01"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	env, rows := igRows(t, out)
	if !env.OK || len(rows) != 3 || env.Meta["ig_user_id"] != "1784" {
		t.Fatal(out.String())
	}
	if rows[0]["reach"] != float64(120) || rows[0]["views"] != float64(340) || rows[0]["media_product_type"] != "REELS" {
		t.Fatalf("reel row: %v", rows[0])
	}
	if e, _ := rows[2]["error"].(map[string]any); e["code"] != "UNSUPPORTED_MEDIA" {
		t.Fatalf("ad row: %v", rows[2])
	}
	if len(env.Warnings) != 1 || env.Warnings[0].Code != "PARTIAL_INSIGHTS" {
		t.Fatalf("warnings: %v", env.Warnings)
	}
}

func TestInstagramMediaInsightsByIDWithBreakdown(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/21":
			_, _ = fmt.Fprint(w, `{"id":"21","media_type":"IMAGE","media_product_type":"STORY"}`)
		case "/v25.0/21/insights":
			q := r.URL.Query()
			if q.Get("metric") != "navigation" || q.Get("breakdown") != "story_navigation_action_type" {
				t.Errorf("query: %v", q)
			}
			_, _ = fmt.Fprint(w, `{"data":[{"name":"navigation","period":"lifetime","total_value":{"value":40,"breakdowns":[{"dimension_keys":["story_navigation_action_type"],"results":[{"dimension_values":["TAP_FORWARD"],"value":30},{"dimension_values":["TAP_EXIT"],"value":10}]}]}}]}`)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "insights", "media", "21", "--profile", "test", "--metric", "navigation", "--breakdown", "story_navigation_action_type", "--raw"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	_, rows := igRows(t, out)
	by, _ := rows[0]["navigation_by_story_navigation_action_type"].(map[string]any)
	if rows[0]["navigation"] != float64(40) || by["TAP_FORWARD"] != float64(30) || rows[0]["insights"] == nil {
		t.Fatal(out.String())
	}
}

func TestInstagramMediaInsightsRowErrors(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/31", "/v25.0/32":
			_, _ = fmt.Fprintf(w, `{"id":"%s","media_product_type":"STORY"}`, r.URL.Path[len("/v25.0/"):])
		case "/v25.0/31/insights":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":10,"message":"(#10) Not enough viewers for the media to show insights"}}`)
		case "/v25.0/32/insights":
			_, _ = fmt.Fprint(w, `{"data":[{"name":"reach","values":[{"value":9}]}]}`)
		}
	})
	if err := executeTest(a, []string{"--config", cfg, "ig", "insights", "media", "31", "32", "--profile", "test"}); err != nil {
		t.Fatal(err, out.String())
	}
	_, rows := igRows(t, out)
	if e, _ := rows[0]["error"].(map[string]any); e["code"] != "ACCESS_DENIED" || rows[1]["reach"] != float64(9) {
		t.Fatal(out.String())
	}

	calls := 0
	a2, out2, _, cfg2 := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v25.0/41/insights" || r.URL.Path == "/v25.0/42/insights" {
			calls++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":190,"message":"Invalid OAuth access token."}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"41","media_product_type":"REELS"}`)
	})
	err := executeTest(a2, []string{"--config", cfg2, "ig", "insights", "media", "41", "42", "--profile", "test"})
	if err == nil || output.AsError(err).Code != "AUTH_INVALID" || calls != 1 {
		t.Fatalf("fatal error did not stop: %v calls=%d %s", err, calls, out2.String())
	}
}

func TestInstagramAccountInsights(t *testing.T) {
	since := strconv.FormatInt(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Unix(), 10)
	until := strconv.FormatInt(time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC).Unix(), 10)
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v25.0/1784/insights" || q.Get("since") != since || q.Get("until") != until || q.Get("period") != "day" || q.Get("metric_type") != "total_value" || q.Get("breakdown") != "media_product_type" || q.Get("metric") != igAccountMetrics {
			t.Errorf("request: %s %v", r.URL.Path, q)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"name":"reach","period":"day","total_value":{"value":500,"breakdowns":[{"dimension_keys":["media_product_type"],"results":[{"dimension_values":["REEL"],"value":300}]}]}},{"name":"views","period":"day","total_value":{"value":900}}]}`)
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "insights", "account", "--profile", "test", "--ig-user", "1784", "--since", "2026-11-01", "--until", "2026-11-03", "--breakdown", "media_product_type"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	env, rows := igRows(t, out)
	by, _ := rows[0]["reach_by_media_product_type"].(map[string]any)
	if !env.OK || len(rows) != 1 || rows[0]["reach"] != float64(500) || rows[0]["views"] != float64(900) || by["REEL"] != float64(300) {
		t.Fatal(out.String())
	}
}

func TestInstagramAccountTimeSeries(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("metric_type") != "time_series" {
			t.Errorf("query: %v", r.URL.Query())
		}
		_, _ = fmt.Fprint(w, `{"data":[{"name":"reach","period":"day","values":[{"value":5,"end_time":"2026-11-02T08:00:00+0000"},{"value":3,"end_time":"2026-11-01T08:00:00+0000"}]}]}`)
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "insights", "account", "--profile", "test", "--ig-user", "1784", "--metric", "reach", "--metric-type", "time_series"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	_, rows := igRows(t, out)
	if len(rows) != 2 || rows[0]["end_time"] != "2026-11-01T08:00:00+0000" || rows[0]["reach"] != float64(3) {
		t.Fatal(out.String())
	}
}

func TestInstagramAudienceInsights(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("metric") != "engaged_audience_demographics" || q.Get("breakdown") != "age,gender" || q.Get("timeframe") != "this_week" || q.Get("period") != "lifetime" {
			t.Errorf("query: %v", q)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"name":"engaged_audience_demographics","total_value":{"breakdowns":[{"dimension_keys":["age","gender"],"results":[{"dimension_values":["18-24","F"],"value":4},{"dimension_values":["25-34","M"],"value":12}]}]}}]}`)
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "insights", "audience", "--profile", "test", "--ig-user", "1784", "--audience", "engaged", "--timeframe", "this_week"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	_, rows := igRows(t, out)
	if len(rows) != 2 || rows[0]["age"] != "25-34" || rows[0]["gender"] != "M" || rows[0]["value"] != float64(12) {
		t.Fatal(out.String())
	}
}

func TestInstagramInsightsValidation(t *testing.T) {
	a, _, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("invalid input made a request: %s", r.URL.Path) })
	for name, args := range map[string][]string{
		"no ig user":         {"media"},
		"ids with stories":   {"media", "21", "--stories"},
		"bad type":           {"media", "--ig-user", "1784", "--type", "igtv"},
		"bad date":           {"media", "--ig-user", "1784", "--since", "11/01/2026"},
		"inverted range":     {"account", "--ig-user", "1784", "--since", "2026-11-05", "--until", "2026-11-01"},
		"half range":         {"account", "--ig-user", "1784", "--since", "2026-11-01"},
		"preset and range":   {"account", "--ig-user", "1784", "--date-preset", "last_7d", "--since", "2026-11-01", "--until", "2026-11-02"},
		"series breakdown":   {"account", "--ig-user", "1784", "--metric", "reach", "--metric-type", "time_series", "--breakdown", "follow_type"},
		"bad preset":         {"account", "--ig-user", "1784", "--date-preset", "last_90d"},
		"bad audience":       {"audience", "--ig-user", "1784", "--audience", "everyone"},
		"bad audience dim":   {"audience", "--ig-user", "1784", "--breakdown", "language"},
		"retired timeframe":  {"audience", "--ig-user", "1784", "--timeframe", "last_30_days"},
		"too many media ids": append([]string{"media"}, make51IDs()...),
	} {
		full := append(append([]string{"--config", cfg, "ig", "insights"}, args...), "--profile", "test")
		err := executeTest(a, full)
		if err == nil || output.AsError(err).Code != "USAGE" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func make51IDs() []string {
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = strconv.Itoa(100 + i)
	}
	return ids
}
