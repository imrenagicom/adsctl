package schema

import "testing"

func TestMutationSchemaRejectsUnsafePayloads(t *testing.T) {
	for _, input := range []string{`{"access_token":"secret"}`, `{"status":"DELETED"}`, `{"name":"x","adset_id":"123","creative":{"object_story_spec":{}}}`, `{"name":"x","daily_budget":2.5}`, `{"name":"x","daily_budget":100,"lifetime_budget":200}`} {
		p, err := Decode([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if Validate("ads", "create", p) == nil {
			t.Fatal("accepted", input)
		}
	}
	for _, input := range []string{`{} {}`, `{} garbage`, `[]`, `null`} {
		if _, err := Decode([]byte(input)); err == nil {
			t.Fatal("accepted", input)
		}
	}
}
func TestExactIntegerPayload(t *testing.T) {
	p, err := Decode([]byte(`{"name":"x","daily_budget":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate("campaigns", "update", p); err != nil {
		t.Fatal(err)
	}
	if p["daily_budget"].(interface{ String() string }).String() != "9007199254740993" {
		t.Fatal(p)
	}
}

func TestObservedNativeSubsetComparison(t *testing.T) {
	want := map[string]any{"daily_budget": "2500", "targeting": map[string]any{"countries": []any{"ID"}}, "creative": map[string]any{"creative_id": "123"}}
	got := map[string]any{"daily_budget": 2500, "targeting": map[string]any{"countries": []any{"ID"}, "provider_default": true}, "creative": map[string]any{"id": "123", "name": "existing"}}
	if !MatchesPayload(want, got) {
		t.Fatal("native normalization rejected")
	}
	got["daily_budget"] = 2000
	if MatchesPayload(want, got) {
		t.Fatal("budget difference accepted")
	}
}

func TestCreativeUpdateIsMetadataOnly(t *testing.T) {
	if err := Validate("creatives", "update", map[string]any{"name": "Launch Video"}); err != nil {
		t.Fatal("metadata name update rejected:", err)
	}
	for _, p := range []map[string]any{
		{"object_story_spec": map[string]any{"page_id": "1"}},
		{"object_story_id": "1_2"},
		{"name": "x", "body": "changed copy"},
	} {
		if err := Validate("creatives", "update", p); err == nil {
			t.Fatal("creative content mutation accepted:", p)
		}
	}
}

func TestUnorderedTargetingFieldsCompareAsSets(t *testing.T) {
	planned := map[string]any{"targeting": map[string]any{
		"instagram_positions": []any{"reels", "story"},
		"publisher_platforms": []any{"facebook", "instagram"},
	}}
	reordered := map[string]any{"targeting": map[string]any{
		"instagram_positions": []any{"story", "reels"},
		"publisher_platforms": []any{"instagram", "facebook"},
	}}
	if path := FirstMismatch("adsets", planned, reordered); path != "" {
		t.Fatal("reordered set fields reported drift at", path)
	}
	// A genuinely changed placement must still be drift, reported at its field.
	changed := map[string]any{"targeting": map[string]any{
		"instagram_positions": []any{"story", "feed"},
		"publisher_platforms": []any{"instagram", "facebook"},
	}}
	if path := FirstMismatch("adsets", planned, changed); path != "targeting.instagram_positions" {
		t.Fatal("changed placement not reported at the field, got", path)
	}
}

func TestOrderedArraysStayPositional(t *testing.T) {
	planned := map[string]any{"adset_schedule": []any{map[string]any{"start": "1"}, map[string]any{"start": "2"}}}
	reordered := map[string]any{"adset_schedule": []any{map[string]any{"start": "2"}, map[string]any{"start": "1"}}}
	if path := FirstMismatch("adsets", planned, reordered); path != "adset_schedule[0].start" {
		t.Fatal("ordered array reordering was not rejected, got", path)
	}
}

func TestMismatchReportsNestedFieldPath(t *testing.T) {
	planned := map[string]any{"targeting": map[string]any{"geo_locations": map[string]any{"countries": []any{"ID"}}}}
	observed := map[string]any{"targeting": map[string]any{"geo_locations": map[string]any{"countries": []any{"US"}}}}
	if path := FirstMismatch("adsets", planned, observed); path != "targeting.geo_locations.countries[0]" {
		t.Fatal(path)
	}
}

func TestCreativeNameProviderSuffixIsNormalized(t *testing.T) {
	planned := map[string]any{"name": "Launch Video", "object_story_spec": map[string]any{"page_id": "1"}}
	observed := map[string]any{"name": "Launch Video - 2026-10-16", "object_story_spec": map[string]any{"page_id": "1"}}
	if path := FirstMismatch("creatives", planned, observed); path != "" {
		t.Fatal("provider name suffix reported drift at", path)
	}
	// A wholly different creative name is still drift.
	observed["name"] = "Something Else"
	if path := FirstMismatch("creatives", planned, observed); path != "name" {
		t.Fatal("different creative name accepted, got", path)
	}
	// Account/asset identity fields stay strict even when the name matches.
	observed["name"] = "Launch Video - 2026-10-16"
	observed["object_story_spec"] = map[string]any{"page_id": "2"}
	if path := FirstMismatch("creatives", planned, observed); path != "object_story_spec.page_id" {
		t.Fatal("asset identity drift accepted, got", path)
	}
	// Other resources keep strict name comparison.
	other := map[string]any{"name": "Launch Video - 2026-10-16"}
	if path := FirstMismatch("campaigns", map[string]any{"name": "Launch Video"}, other); path != "name" {
		t.Fatal("campaign name normalized, got", path)
	}
}

func TestApplyTextOptionsAndCloneCreative(t *testing.T) {
	base, err := Decode([]byte(`{
		"name": "Story Ad Creative",
		"object_story_spec": {
			"page_id": "101",
			"instagram_user_id": "202",
			"link_data": {
				"image_hash": "abc123",
				"link": "https://example.com/app",
				"message": "Old single primary text",
				"name": "Old single headline",
				"call_to_action": {"type": "LEARN_MORE", "value": {"link": "https://example.com/app"}}
			}
		},
		"url_tags": "utm_source=ig"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	primary := []string{"Opt 4 first", "Opt 1", "Opt 2", "Opt 3", "Opt 5"}
	headlines := []string{"Headline 1", "Headline 2", "Headline 3"}
	cloned, err := CloneCreativeForTextOptions(base, "Story Ad Creative v4", primary, headlines)
	if err != nil {
		t.Fatal(err)
	}
	story := cloned["object_story_spec"].(map[string]any)
	linkData := story["link_data"].(map[string]any)
	if linkData["message"] != nil || linkData["name"] != nil {
		t.Fatalf("expected link_data message/name to be removed, got %v", linkData)
	}
	if linkData["image_hash"] != "abc123" || linkData["link"] != "https://example.com/app" {
		t.Fatalf("expected link_data media/link preserved, got %v", linkData)
	}
	afs := cloned["asset_feed_spec"].(map[string]any)
	bodies := afs["bodies"].([]any)
	titles := afs["titles"].([]any)
	if len(bodies) != 5 || bodies[0].(map[string]any)["text"] != "Opt 4 first" || len(titles) != 3 {
		t.Fatalf("unexpected asset_feed_spec: %v", afs)
	}
	// Verify provider response with id fields on bodies/titles matches planned payload
	observed := map[string]any{
		"id":                "999",
		"account_id":        "123",
		"name":              "Story Ad Creative v4 - 2026-10-20",
		"object_story_spec": story,
		"url_tags":          "utm_source=ig",
		"asset_feed_spec": map[string]any{
			"ad_formats":        []any{"AUTOMATIC_FORMAT"},
			"optimization_type": "DEGREES_OF_FREEDOM",
			"bodies": []any{
				map[string]any{"id": "1", "text": "Opt 4 first"},
				map[string]any{"id": "2", "text": "Opt 1"},
				map[string]any{"id": "3", "text": "Opt 2"},
				map[string]any{"id": "4", "text": "Opt 3"},
				map[string]any{"id": "5", "text": "Opt 5"},
			},
			"titles": []any{
				map[string]any{"id": "6", "text": "Headline 1"},
				map[string]any{"id": "7", "text": "Headline 2"},
				map[string]any{"id": "8", "text": "Headline 3"},
			},
		},
	}
	if path := FirstMismatch("creatives", cloned, observed); path != "" {
		t.Fatalf("expected observed creative to match planned payload, got mismatch at %s", path)
	}
	// Cloning from an already multi-text observed creative while overriding only primaryTexts preserves normalized titles.
	recloned, err := CloneCreativeForTextOptions(observed, "Story Ad Creative v5", []string{"New Opt 1", "New Opt 2"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reAFS := recloned["asset_feed_spec"].(map[string]any)
	reTitles := reAFS["titles"].([]any)
	if len(reTitles) != 3 || reTitles[0].(map[string]any)["id"] != nil {
		t.Fatalf("expected provider id stripped from preserved titles, got %v", reTitles)
	}
	// Reject >5 options or blank strings.
	if err := ApplyTextOptions(map[string]any{}, []string{"1", "2", "3", "4", "5", "6"}, nil); err == nil {
		t.Fatal("expected >5 primary texts to fail")
	}
	if err := ApplyTextOptions(map[string]any{}, []string{"   "}, nil); err == nil {
		t.Fatal("expected blank primary text to fail")
	}
}
