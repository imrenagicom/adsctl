package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifest = `version: 1
kind: meta-launch
name: traffic
campaigns:
  - key: main
    payload:
      name: Traffic
      objective: OUTCOME_TRAFFIC
      special_ad_categories: []
adsets:
  - key: audience
    campaign_ref: main
    payload:
      name: Audience
      daily_budget: 9007199254740993
      billing_event: IMPRESSIONS
      optimization_goal: LINK_CLICKS
      targeting:
        geo_locations:
          countries: [ID]
creatives:
  - key: post
    payload:
      name: Existing post
      object_story_id: '123_456'
ads:
  - key: ad
    adset_ref: audience
    creative_ref: post
    payload:
      name: Ad
`

func TestPlanResolvesReferencesAndPreservesIntegers(t *testing.T) {
	p, err := Compile([]byte(manifest), t.TempDir(), Scope{Account: "act_123"})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Validate(true); err != nil {
		t.Fatal(err)
	}
	if p.Steps[0].Node.Payload["status"] != "PAUSED" {
		t.Fatal(p)
	}
	payload, err := Payload(p.Steps[1], map[string]string{"campaigns.main": "999"})
	if err != nil || payload["campaign_id"] != "999" || payload["daily_budget"].(interface{ String() string }).String() != "9007199254740993" {
		t.Fatal(err, payload)
	}
	p.Steps[0].Node.Payload["name"] = "changed"
	if p.Validate(true) == nil {
		t.Fatal("modified plan accepted")
	}
}
func TestPlanRejectsConflictsAndActiveCreates(t *testing.T) {
	for _, bad := range []string{
		strings.Replace(manifest, "campaign_ref: main", "campaign_ref: missing", 1),
		strings.Replace(manifest, "name: Audience", "name: Audience\n      campaign_id: '555'", 1),
		strings.Replace(manifest, "name: Traffic", "name: Traffic\n      status: ACTIVE", 1),
		strings.Replace(manifest, "creative_ref: post", "creative_ref: audience", 1),
		strings.Replace(manifest, "version: 1", "version: 2", 1),
	} {
		if _, err := Compile([]byte(bad), t.TempDir(), Scope{}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}
func TestPlanDetectsChangedAssets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "banner.jpg")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := strings.Replace(manifest, "campaigns:", "assets:\n  - key: banner\n    type: image\n    file: banner.jpg\ncampaigns:", 1)
	p, err := Compile([]byte(raw), dir, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.CheckFiles(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if p.CheckFiles() == nil {
		t.Fatal("changed media accepted")
	}
}
func TestPlanSupportsCreativePrimaryTextsAndHeadlines(t *testing.T) {
	multiTextManifest := strings.Replace(manifest,
		"      object_story_id: '123_456'",
		"      object_story_spec:\n        page_id: '101'\n        link_data:\n          image_hash: abc\n          link: https://example.com\n    primary_texts:\n      - Primary 1\n      - Primary 2\n    headlines:\n      - Headline 1",
		1,
	)
	p, err := Compile([]byte(multiTextManifest), t.TempDir(), Scope{Account: "act_123"})
	if err != nil {
		t.Fatal(err)
	}
	step, ok := p.Step("creatives.post")
	if !ok {
		t.Fatal("missing creative step")
	}
	payload, err := Payload(step, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	afs, ok := payload["asset_feed_spec"].(map[string]any)
	if !ok || len(afs["bodies"].([]any)) != 2 || len(afs["titles"].([]any)) != 1 {
		t.Fatalf("unexpected asset_feed_spec in compiled payload: %v", payload)
	}
}

func TestPlanSupportsAdvantageBudgetAndPlacementsAndRejectsAdsetBudgetConflict(t *testing.T) {
	advManifest := `version: 1
kind: meta-launch
name: advantage-launch
campaigns:
  - key: main
    advantage_budget: true
    payload:
      name: Sales Campaign
      objective: OUTCOME_SALES
      daily_budget: 100000
      special_ad_categories: []
adsets:
  - key: audience
    campaign_ref: main
    advantage_placements: true
    payload:
      name: ID 18+ Audience
      billing_event: IMPRESSIONS
      optimization_goal: OFFSITE_CONVERSIONS
      targeting:
        geo_locations:
          countries: [ID]
        publisher_platforms: [instagram]
        instagram_positions: [reels, story]
creatives:
  - key: post
    payload:
      name: Existing post
      object_story_id: '123_456'
ads:
  - key: ad
    adset_ref: audience
    creative_ref: post
    payload:
      name: Ad
`
	p, err := Compile([]byte(advManifest), t.TempDir(), Scope{Account: "act_123"})
	if err != nil {
		t.Fatal(err)
	}
	campStep, _ := p.Step("campaigns.main")
	campPayload, err := Payload(campStep, map[string]string{})
	if err != nil || campPayload["bid_strategy"] != "LOWEST_COST_WITHOUT_CAP" || campPayload["is_adset_budget_sharing_enabled"] != false {
		t.Fatalf("unexpected campaign payload: %v %v", err, campPayload)
	}
	adsetStep, _ := p.Step("adsets.audience")
	adsetPayload, err := Payload(adsetStep, map[string]string{"campaigns.main": "777"})
	if err != nil {
		t.Fatal(err)
	}
	targ := adsetPayload["targeting"].(map[string]any)
	if targ["publisher_platforms"] != nil || targ["instagram_positions"] != nil {
		t.Fatalf("expected manual placements stripped for advantage_placements: %v", targ)
	}
	// Conflicting ad-set daily_budget when parent campaign uses Advantage+ budget must fail.
	conflict := strings.Replace(advManifest, "      billing_event: IMPRESSIONS", "      daily_budget: 50000\n      billing_event: IMPRESSIONS", 1)
	if _, err := Compile([]byte(conflict), t.TempDir(), Scope{Account: "act_123"}); err == nil {
		t.Fatal("expected adset daily_budget under Advantage+ campaign budget to fail")
	}
}

func TestPlanSupportsSourceInstagramMediaIDCreative(t *testing.T) {
	igPostManifest := strings.Replace(manifest,
		"      object_story_id: '123_456'",
		"      object_id: '1336593769543062'\n      instagram_user_id: '17841451279536743'\n      source_instagram_media_id: '18636447112018423'\n      call_to_action:\n        type: LEARN_MORE\n        value:\n          link: https://example.com",
		1,
	)
	p, err := Compile([]byte(igPostManifest), t.TempDir(), Scope{Account: "act_123"})
	if err != nil {
		t.Fatal(err)
	}
	step, ok := p.Step("creatives.post")
	if !ok {
		t.Fatal("missing creative step")
	}
	payload, err := Payload(step, map[string]string{})
	if err != nil || payload["source_instagram_media_id"] != "18636447112018423" || payload["object_id"] != "1336593769543062" {
		t.Fatalf("unexpected existing IG post creative payload: %v %v", err, payload)
	}
	// Missing object_id with source_instagram_media_id must fail during compile.
	badManifest := strings.Replace(igPostManifest, "      object_id: '1336593769543062'\n", "", 1)
	if _, err := Compile([]byte(badManifest), t.TempDir(), Scope{Account: "act_123"}); err == nil {
		t.Fatal("expected missing object_id with source_instagram_media_id to fail")
	}
}
