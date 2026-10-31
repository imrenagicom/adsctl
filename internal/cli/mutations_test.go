package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"adsctl/internal/output"
)

func TestMutationDefaultsPausedAndVerifies(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("status") != "PAUSED" || r.Form.Get("special_ad_categories") != "[]" {
				t.Errorf("unsafe defaults: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"456"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"456","account_id":"123","status":"PAUSED","effective_status":"PAUSED","name":"test","objective":"OUTCOME_TRAFFIC","special_ad_categories":[]}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err != nil || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}
}
func TestWritesRequireExplicitScopeAndDryRunDoesNotWrite(t *testing.T) {
	t.Setenv("ADSCTL_PROFILE", "test")
	for _, dry := range []bool{false, true} {
		posts := 0
		a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				posts++
			}
			_, _ = fmt.Fprint(w, `{"id":"456","account_id":"123"}`)
		})
		args := []string{"--config", path, "meta", "campaigns", "pause", "456", "--account", "123"}
		if dry {
			args = append(args, "--profile", "test", "--dry-run")
		}
		err := executeTest(a, args)
		if posts != 0 {
			t.Fatal("unexpected write")
		}
		if dry {
			if err != nil || !bytes.Contains(out.Bytes(), []byte(`"state": "planned"`)) {
				t.Fatal(err, out.String())
			}
		} else if err == nil || output.AsError(err).Code != "WRITE_SCOPE" {
			t.Fatal(err)
		}
	}
}
func TestAmbiguousCreateIsNotRetried(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) { posts++; _, _ = fmt.Fprint(w, `unreadable`) })
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err == nil || output.AsError(err).Exit != 10 || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "unknown"`)) {
		t.Fatal(err, posts, out.String())
	}
}
func TestCrossAccountWriteBlocked(t *testing.T) {
	posts := 0
	a, _, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			posts++
		}
		_, _ = fmt.Fprint(w, `{"id":"456","account_id":"999"}`)
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "activate", "456", "--profile", "test", "--account", "123"})
	if err == nil || output.AsError(err).Exit != 4 || posts != 0 {
		t.Fatal(err, posts)
	}
}
func TestVerificationFailurePreservesAppliedReceipt(t *testing.T) {
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = fmt.Fprint(w, `{"id":"456"}`)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"code":100,"message":"unavailable field"}}`)
		}
	})
	err := executeTest(a, []string{"--config", path, "meta", "campaigns", "create", "--profile", "test", "--account", "123", "--name", "test", "--objective", "OUTCOME_TRAFFIC", "--special-ad-categories", "NONE"})
	if err == nil || output.AsError(err).Exit != 9 || output.AsError(err).HTTPStatus != http.StatusBadRequest || !bytes.Contains(out.Bytes(), []byte(`"id": "456"`)) || !bytes.Contains(out.Bytes(), []byte(`"state": "applied_unverified"`)) || !bytes.Contains(out.Bytes(), []byte(`"verification_error"`)) {
		t.Fatal(err, out.String())
	}
}

func TestStatusMutationsDoNotDuplicateVerificationFields(t *testing.T) {
	for _, args := range [][]string{
		{"meta", "ads", "create", "--profile", "test", "--account", "123", "--name", "ad-1", "--adset", "200", "--creative", "300", "--status", "PAUSED"},
		{"meta", "campaigns", "pause", "456", "--profile", "test", "--account", "123"},
		{"meta", "campaigns", "activate", "456", "--profile", "test", "--account", "123"},
		{"meta", "campaigns", "archive", "456", "--profile", "test", "--account", "123"},
		{"meta", "campaigns", "update", "456", "--profile", "test", "--account", "123", "--status", "PAUSED"},
	} {
		a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				_, _ = fmt.Fprint(w, `{"id":"456","success":true}`)
				return
			}
			status := "PAUSED"
			for _, arg := range args {
				switch arg {
				case "activate":
					status = "ACTIVE"
				case "archive":
					status = "ARCHIVED"
				}
			}
			_, _ = fmt.Fprintf(w, `{"id":"456","account_id":"123","adset_id":"200","creative":{"id":"300"},"name":"ad-1","status":%q,"effective_status":%q}`, status, status)
		})
		if err := executeTest(a, append([]string{"--config", path}, args...)); err != nil || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
			t.Fatal(args, err, out.String())
		}
	}
}

func TestCreativeMetadataNameUpdateRecoversNormalizedName(t *testing.T) {
	posts := 0
	name := "Launch Video - 2026-10-16"
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			name = r.Form.Get("name")
			_, _ = fmt.Fprint(w, `{"success":true}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"456","account_id":"123","name":%q}`, name)
	})
	// The provider stored a suffixed name at create; an explicit metadata update
	// is the recovery path, without recreating the accepted creative.
	err := executeTest(a, []string{"--config", path, "meta", "creatives", "update", "456", "--profile", "test", "--account", "123", "--name", "Launch Video"})
	if err != nil || posts != 1 || name != "Launch Video" || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, name, out.String())
	}
}

func TestBudgetCurrencyAndObservedAmount(t *testing.T) {
	for _, scenario := range []string{"success", "currency", "campaign-owned", "not-converged"} {
		t.Run(scenario, func(t *testing.T) {
			posts := 0
			amount := "1000"
			a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("daily_budget") != "9007199254740993" {
						t.Error("budget lost precision", r.Form)
					}
					if scenario != "not-converged" {
						amount = r.Form.Get("daily_budget")
					}
					_, _ = fmt.Fprint(w, `{"success":true}`)
					return
				}
				switch r.URL.Path {
				case "/v25.0/act_123":
					currency := "USD"
					if scenario == "currency" {
						currency = "IDR"
					}
					_, _ = fmt.Fprintf(w, `{"id":"act_123","currency":%q}`, currency)
				case "/v25.0/100":
					budget := "0"
					if scenario == "campaign-owned" {
						budget = "1000"
					}
					_, _ = fmt.Fprintf(w, `{"id":"100","account_id":"123","daily_budget":%q}`, budget)
				default:
					_, _ = fmt.Fprintf(w, `{"id":"456","account_id":"123","campaign_id":"100","daily_budget":%q}`, amount)
				}
			})
			err := executeTest(a, []string{"--config", path, "meta", "adsets", "budget", "set", "456", "--profile", "test", "--account", "123", "--daily-budget-minor", "9007199254740993", "--currency", "USD"})
			switch scenario {
			case "success":
				if err != nil || posts != 1 {
					t.Fatal(err, posts, out.String())
				}
			case "not-converged":
				if err == nil || output.AsError(err).Exit != 9 || posts != 1 {
					t.Fatal(err, posts, out.String())
				}
			default:
				if err == nil || posts != 0 {
					t.Fatal(err, posts)
				}
			}
		})
	}
}

func TestAdsTextSetClonesCreativeAndUpdatesAd(t *testing.T) {
	posts := 0
	currentCreativeID := "300"
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			switch r.URL.Path {
			case "/v25.0/act_123/adcreatives":
				if !bytes.Contains([]byte(r.Form.Get("asset_feed_spec")), []byte(`"Opt 4"`)) {
					t.Errorf("missing expected primary text in asset_feed_spec: %s", r.Form.Get("asset_feed_spec"))
				}
				_, _ = fmt.Fprint(w, `{"id":"301"}`)
				return
			case "/v25.0/456":
				currentCreativeID = "301"
				_, _ = fmt.Fprint(w, `{"success":true}`)
				return
			}
		}
		switch r.URL.Path {
		case "/v25.0/456":
			_, _ = fmt.Fprintf(w, `{"id":"456","account_id":"123","name":"Ad 1","status":"ACTIVE","effective_status":"ACTIVE","creative":{"id":%q}}`, currentCreativeID)
		case "/v25.0/300":
			_, _ = fmt.Fprint(w, `{"id":"300","account_id":"123","name":"Creative v1","object_story_spec":{"page_id":"10","link_data":{"image_hash":"h1","link":"https://example.com","message":"old","name":"old title","call_to_action":{"type":"LEARN_MORE","value":{"link":"https://example.com"}}}},"url_tags":"utm_source=ig"}`)
		case "/v25.0/301":
			_, _ = fmt.Fprint(w, `{"id":"301","account_id":"123","name":"Creative v2","object_story_spec":{"page_id":"10"},"url_tags":"utm_source=ig","degrees_of_freedom_spec":{"creative_features_spec":{"text_optimizations":{"enroll_status":"OPT_IN"}}},"asset_feed_spec":{"ad_formats":["SINGLE_IMAGE"],"optimization_type":"REGULAR","images":[{"hash":"h1"}],"link_urls":[{"website_url":"https://example.com"}],"call_to_action_types":["LEARN_MORE"],"bodies":[{"id":"1","text":"Opt 4"},{"id":"2","text":"Opt 1"}],"titles":[{"id":"3","text":"Head 1"}]}}`)
		}
	})
	err := executeTest(a, []string{
		"--config", path,
		"meta", "ads", "text", "set", "456",
		"--profile", "test", "--account", "123",
		"--name", "Creative v2",
		"--primary-text", "Opt 4",
		"--primary-text", "Opt 1",
		"--headline", "Head 1",
	})
	if err != nil || posts != 2 || !bytes.Contains(out.Bytes(), []byte(`"creative_id": "301"`)) || !bytes.Contains(out.Bytes(), []byte(`"previous_creative_id": "300"`)) {
		t.Fatal(err, posts, out.String())
	}
}

func TestCampaignAdvantageBudgetAndAdsetPlacementsSet(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			switch r.URL.Path {
			case "/v25.0/act_123/campaigns":
				if r.Form.Get("daily_budget") != "100000" || r.Form.Get("bid_strategy") != "LOWEST_COST_WITHOUT_CAP" || r.Form.Get("is_adset_budget_sharing_enabled") != "false" {
					t.Errorf("unexpected Advantage+ campaign form: %v", r.Form)
				}
				_, _ = fmt.Fprint(w, `{"id":"700"}`)
				return
			case "/v25.0/800":
				if bytes.Contains([]byte(r.Form.Get("targeting")), []byte("publisher_platforms")) || !bytes.Contains([]byte(r.Form.Get("targeting")), []byte("6002950107774")) {
					t.Errorf("expected manual placements stripped and interests preserved: %s", r.Form.Get("targeting"))
				}
				_, _ = fmt.Fprint(w, `{"success":true}`)
				return
			}
		}
		switch r.URL.Path {
		case "/v25.0/700":
			_, _ = fmt.Fprint(w, `{"id":"700","account_id":"123","name":"Adv+ Campaign","objective":"OUTCOME_SALES","daily_budget":"100000","bid_strategy":"LOWEST_COST_WITHOUT_CAP","is_adset_budget_sharing_enabled":false,"special_ad_categories":[],"status":"PAUSED","effective_status":"PAUSED"}`)
		case "/v25.0/800":
			if posts >= 2 {
				_, _ = fmt.Fprint(w, `{"id":"800","account_id":"123","name":"Adset 1","status":"ACTIVE","effective_status":"ACTIVE","placement_soft_opt_out":false,"targeting":{"age_min":18,"age_max":65,"geo_locations":{"countries":["ID"]},"flexible_spec":[{"interests":[{"id":"6002950107774","name":"Chinese language"}]}]}}`)
			} else {
				_, _ = fmt.Fprint(w, `{"id":"800","account_id":"123","name":"Adset 1","status":"ACTIVE","effective_status":"ACTIVE","targeting":{"age_min":18,"age_max":65,"geo_locations":{"countries":["ID"]},"flexible_spec":[{"interests":[{"id":"6002950107774","name":"Chinese language"}]}],"publisher_platforms":["instagram"],"instagram_positions":["stream","story","reels"]}}`)
			}
		}
	})
	err := executeTest(a, []string{
		"--config", path,
		"meta", "campaigns", "create",
		"--profile", "test", "--account", "123",
		"--name", "Adv+ Campaign",
		"--objective", "OUTCOME_SALES",
		"--special-ad-categories", "NONE",
		"--advantage-budget",
		"--daily-budget-minor", "100000",
	})
	if err != nil || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}

	out.Reset()
	err = executeTest(a, []string{
		"--config", path,
		"meta", "adsets", "placements", "set", "800",
		"--profile", "test", "--account", "123",
		"--advantage-plus",
		"--placement-soft-opt-out", "false",
	})
	if err != nil || posts != 2 || !bytes.Contains(out.Bytes(), []byte(`"placements_mode": "advantage_plus"`)) || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}
}

func TestCreativesCreateWithSourceInstagramMediaID(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("source_instagram_media_id") != "18636447112018423" || r.Form.Get("instagram_user_id") != "17841451279536743" || r.Form.Get("object_id") != "1336593769543062" {
				t.Errorf("unexpected creative form: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"901"}`)
			return
		}
		// Simulate Meta returning actor_id instead of object_id and appending a suffix to name.
		_, _ = fmt.Fprint(w, `{"id":"901","account_id":"123","name":"Boost IG post 2026-10-22-abc123","actor_id":"1336593769543062","instagram_user_id":"17841451279536743","source_instagram_media_id":"18636447112018423","call_to_action":{"type":"LEARN_MORE","value":{"link":"https://example.com"}}}`)
	})
	payload := `{"name":"Boost IG post","object_id":"1336593769543062","instagram_user_id":"17841451279536743","source_instagram_media_id":"18636447112018423","call_to_action":{"type":"LEARN_MORE","value":{"link":"https://example.com"}}}`
	a.In = bytes.NewBufferString(payload)
	if err := executeTest(a, []string{"--config", path, "meta", "creatives", "validate", "--operation", "create", "--file", "-", "--profile", "test", "--account", "123"}); err != nil || !bytes.Contains(out.Bytes(), []byte(`"local_valid": true`)) {
		t.Fatal(err, out.String())
	}
	out.Reset()
	a.In = bytes.NewBufferString(payload)
	if err := executeTest(a, []string{"--config", path, "meta", "creatives", "create", "--file", "-", "--profile", "test", "--account", "123", "--dry-run"}); err != nil || posts != 0 || !bytes.Contains(out.Bytes(), []byte(`"state": "planned"`)) {
		t.Fatal(err, posts, out.String())
	}
	out.Reset()
	a.In = bytes.NewBufferString(payload)
	if err := executeTest(a, []string{"--config", path, "meta", "creatives", "create", "--file", "-", "--profile", "test", "--account", "123"}); err != nil || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}
}

func TestAudiencesCreateVerifiesWithoutStatusOrRuleEcho(t *testing.T) {
	posts := 0
	a, out, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			if r.URL.Path != "/v25.0/act_123/customaudiences" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("status") != "" || r.Form.Get("prefill") != "true" || !bytes.Contains([]byte(r.Form.Get("rule")), []byte(`"type":"pixel"`)) {
				t.Errorf("unexpected audience form: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"777"}`)
			return
		}
		if bytes.Contains([]byte(r.URL.Query().Get("fields")), []byte("prefill")) || bytes.Contains([]byte(r.URL.Query().Get("fields")), []byte("effective_status")) {
			t.Errorf("unreadable audience fields requested: %s", r.URL.Query().Get("fields"))
		}
		// Meta echoes the rule as an encoded string.
		_, _ = fmt.Fprint(w, `{"id":"777","account_id":"123","name":"Web visitors 180d","rule":"{\"inclusions\":{}}","approximate_count_lower_bound":-1,"operation_status":{"code":200}}`)
	})
	payload := `{"name":"Web visitors 180d","prefill":true,"rule":{"inclusions":{"operator":"or","rules":[{"event_sources":[{"id":"1855259662305866","type":"pixel"}],"retention_seconds":15552000}]}}}`
	a.In = bytes.NewBufferString(payload)
	if err := executeTest(a, []string{"--config", path, "meta", "audiences", "create", "--file", "-", "--profile", "test", "--account", "123", "--dry-run"}); err != nil || posts != 0 || !bytes.Contains(out.Bytes(), []byte(`"state": "planned"`)) {
		t.Fatal(err, posts, out.String())
	}
	out.Reset()
	a.In = bytes.NewBufferString(payload)
	if err := executeTest(a, []string{"--config", path, "meta", "audiences", "create", "--file", "-", "--profile", "test", "--account", "123"}); err != nil || posts != 1 || !bytes.Contains(out.Bytes(), []byte(`"state": "verified"`)) {
		t.Fatal(err, posts, out.String())
	}
}

func TestAudiencesHaveNoStatusOperations(t *testing.T) {
	a, _, _, path := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":"777","account_id":"123"}`)
	})
	for _, op := range []string{"pause", "activate", "archive"} {
		if err := executeTest(a, []string{"--config", path, "meta", "audiences", op, "777", "--profile", "test", "--account", "123", "--dry-run"}); err == nil {
			t.Fatal("audiences accepted", op)
		}
	}
}
