// Package schema defines the deliberately supported native mutation subset.
package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"adsctl/internal/config"
	"adsctl/internal/output"
)

type Field struct {
	Type     string   `json:"type"`
	Required bool     `json:"required_on_create,omitempty"`
	Enum     []string `json:"enum,omitempty"`
}

func Fields(resource, operation string) map[string]Field {
	if operation != "create" && operation != "update" {
		return nil
	}
	common := map[string]string{"name": "string", "adlabels": "array"}
	switch resource {
	case "campaigns":
		common["status"] = "string"
		for _, k := range strings.Fields("objective buying_type bid_strategy start_time stop_time") {
			common[k] = "string"
		}
		for _, k := range strings.Fields("daily_budget lifetime_budget spend_cap") {
			common[k] = "integer"
		}
		for _, k := range strings.Fields("special_ad_categories special_ad_category_country pacing_type") {
			common[k] = "array"
		}
		common["is_adset_budget_sharing_enabled"] = "boolean"
	case "adsets":
		for _, k := range strings.Fields("status billing_event optimization_goal bid_strategy start_time end_time destination_type dsa_beneficiary dsa_payor") {
			common[k] = "string"
		}
		for _, k := range strings.Fields("daily_budget lifetime_budget bid_amount daily_min_spend_target daily_spend_cap lifetime_min_spend_target lifetime_spend_cap") {
			common[k] = "integer"
		}
		for _, k := range strings.Fields("targeting promoted_object bid_constraints") {
			common[k] = "object"
		}
		for _, k := range strings.Fields("attribution_spec pacing_type frequency_control_specs adset_schedule") {
			common[k] = "array"
		}
		common["is_dynamic_creative"] = "boolean"
		if operation == "create" {
			common["campaign_id"] = "id"
		}
	case "ads":
		for _, k := range strings.Fields("status conversion_domain ad_schedule_start_time ad_schedule_end_time") {
			common[k] = "string"
		}
		common["creative"] = "object"
		common["tracking_specs"] = "object"
		if operation == "create" {
			common["adset_id"] = "id"
		}
	case "creatives":
		if operation == "create" {
			for _, k := range strings.Fields("object_story_id image_hash image_url title body link_url object_url url_tags instagram_user_id") {
				common[k] = "string"
			}
			for _, k := range strings.Fields("object_story_spec asset_feed_spec degrees_of_freedom_spec call_to_action contextual_multi_ads") {
				common[k] = "object"
			}
		} else {
			// Update is metadata-only: an accepted creative's content is frozen,
			// so only its label fields remain writable. This is the recovery path
			// for a provider-normalized name; it never recreates the creative.
			common["name"] = "string"
			common["adlabels"] = "array"
		}
	default:
		return nil
	}
	required := map[string][]string{"campaigns": {"name", "objective", "special_ad_categories"}, "adsets": {"name", "campaign_id", "billing_event", "optimization_goal", "targeting"}, "ads": {"name", "adset_id", "creative"}, "creatives": {"name"}}
	out := map[string]Field{}
	for k, v := range common {
		f := Field{Type: v}
		if k == "status" {
			f.Enum = []string{"ACTIVE", "PAUSED"}
			if operation == "update" {
				f.Enum = append(f.Enum, "ARCHIVED", "DELETED")
			}
		}
		if k == "objective" {
			f.Enum = []string{"OUTCOME_AWARENESS", "OUTCOME_ENGAGEMENT", "OUTCOME_LEADS", "OUTCOME_SALES", "OUTCOME_TRAFFIC", "OUTCOME_APP_PROMOTION"}
		}
		out[k] = f
	}
	if operation == "create" {
		for _, k := range required[resource] {
			f := out[k]
			f.Required = true
			out[k] = f
		}
	}
	return out
}
func Validate(resource, operation string, p map[string]any) error {
	fields := Fields(resource, operation)
	if fields == nil || (operation != "create" && operation != "update") {
		return output.Fail("SCHEMA_UNSUPPORTED", "Unsupported mutation schema.", 2)
	}
	if len(p) == 0 {
		return output.Fail("VALIDATION", "Payload must contain fields.", 5)
	}
	for k, v := range p {
		f, ok := fields[k]
		if !ok {
			return output.Fail("VALIDATION", "Unsupported writable field: "+k, 5)
		}
		valid := false
		switch f.Type {
		case "string":
			s, ok := v.(string)
			valid = ok && s != ""
		case "id":
			valid = config.ID(fmt.Sprint(v)) == nil
		case "integer":
			s := fmt.Sprint(v)
			valid = s != ""
			for _, c := range s {
				if c < '0' || c > '9' {
					valid = false
				}
			}
		case "object":
			_, valid = v.(map[string]any)
		case "array":
			_, valid = v.([]any)
		case "boolean":
			_, valid = v.(bool)
		}
		if len(f.Enum) > 0 {
			allowed := false
			for _, value := range f.Enum {
				if v == value {
					allowed = true
				}
			}
			valid = valid && allowed
		}
		if !valid {
			return output.Fail("VALIDATION", fmt.Sprintf("%s requires %s.", k, f.Type), 5)
		}
	}
	for k, f := range fields {
		if f.Required && p[k] == nil {
			return output.Fail("VALIDATION", "Required create field: "+k, 5)
		}
	}
	if s, ok := p["status"].(string); ok {
		allowed := s == "ACTIVE" || s == "PAUSED" || (operation == "update" && (s == "ARCHIVED" || s == "DELETED"))
		if !allowed {
			return output.Fail("VALIDATION", "Unsupported status.", 5)
		}
	}
	if p["daily_budget"] != nil && p["lifetime_budget"] != nil {
		return output.Fail("VALIDATION", "Choose one budget type.", 5)
	}
	if obj, ok := p["creative"].(map[string]any); ok {
		if len(obj) != 1 || config.ID(fmt.Sprint(obj["creative_id"])) != nil {
			return output.Fail("VALIDATION", "creative must reference exactly one existing creative_id.", 5)
		}
	}
	if operation == "create" && resource == "creatives" && p["object_story_spec"] == nil && p["object_story_id"] == nil && p["asset_feed_spec"] == nil {
		return output.Fail("VALIDATION", "Creative requires object_story_spec, object_story_id or asset_feed_spec.", 5)
	}
	return nil
}
func Keys(p map[string]any) string {
	keys := []string{"id", "account_id"}
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
func Defaults(resource string, p map[string]any) {
	if resource != "creatives" && p["status"] == nil {
		p["status"] = "PAUSED"
	}
}
func Decode(b []byte) (map[string]any, error) {
	var p map[string]any
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	if err := d.Decode(&p); err != nil || p == nil {
		return nil, output.Fail("VALIDATION", "Expected a native JSON object.", 5)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, output.Fail("VALIDATION", "Only one JSON value is allowed.", 5)
	}
	return p, nil
}

// ApplyTextOptions configures 1 to 5 primary text options (asset_feed_spec.bodies)
// and/or 1 to 5 headline options (asset_feed_spec.titles) on a creative payload,
// migrating single-media link_data/video_data fields into a complete REGULAR
// asset_feed_spec and opting in to degrees_of_freedom_spec text_optimizations so
// Meta Ads Manager UI displays all text option boxes.
func ApplyTextOptions(p map[string]any, primaryTexts, headlines []string) error {
	if len(primaryTexts) > 5 || len(headlines) > 5 {
		return output.Fail("VALIDATION", "Primary text and headline options must be 1 to 5 non-empty strings.", 5)
	}
	for _, list := range [][]string{primaryTexts, headlines} {
		for _, s := range list {
			if strings.TrimSpace(s) == "" {
				return output.Fail("VALIDATION", "Primary text and headline options must be 1 to 5 non-empty strings.", 5)
			}
		}
	}
	if len(primaryTexts) == 0 && len(headlines) == 0 {
		return nil
	}
	feed, _ := p["asset_feed_spec"].(map[string]any)
	if feed == nil {
		feed = map[string]any{}
	}
	if existingBodies := normalizeTextItems(feed["bodies"]); len(existingBodies) > 0 {
		feed["bodies"] = existingBodies
	}
	if existingTitles := normalizeTextItems(feed["titles"]); len(existingTitles) > 0 {
		feed["titles"] = existingTitles
	}
	if story, ok := p["object_story_spec"].(map[string]any); ok {
		if data, ok := story["link_data"].(map[string]any); ok {
			if len(primaryTexts) == 0 && feed["bodies"] == nil {
				if msg, ok := data["message"].(string); ok && strings.TrimSpace(msg) != "" {
					feed["bodies"] = []any{map[string]any{"text": msg}}
				}
			}
			if len(headlines) == 0 && feed["titles"] == nil {
				if title, ok := data["name"].(string); ok && strings.TrimSpace(title) != "" {
					feed["titles"] = []any{map[string]any{"text": title}}
				}
			}
			if feed["images"] == nil {
				if hash, ok := data["image_hash"].(string); ok && strings.TrimSpace(hash) != "" {
					feed["images"] = []any{map[string]any{"hash": hash}}
				}
			}
			if feed["ad_formats"] == nil && feed["images"] != nil {
				feed["ad_formats"] = []any{"SINGLE_IMAGE"}
			}
			linkURL, ctaType := extractLinkAndCTA(data)
			if feed["link_urls"] == nil && linkURL != "" {
				feed["link_urls"] = []any{map[string]any{"website_url": linkURL}}
			}
			if feed["call_to_action_types"] == nil && ctaType != "" {
				feed["call_to_action_types"] = []any{ctaType}
			}
			if feed["images"] != nil && feed["link_urls"] != nil {
				delete(story, "link_data")
			} else {
				delete(data, "message")
				delete(data, "name")
			}
		}
		if data, ok := story["video_data"].(map[string]any); ok {
			if len(primaryTexts) == 0 && feed["bodies"] == nil {
				if msg, ok := data["message"].(string); ok && strings.TrimSpace(msg) != "" {
					feed["bodies"] = []any{map[string]any{"text": msg}}
				}
			}
			if len(headlines) == 0 && feed["titles"] == nil {
				if title, ok := data["title"].(string); ok && strings.TrimSpace(title) != "" {
					feed["titles"] = []any{map[string]any{"text": title}}
				}
			}
			if feed["videos"] == nil {
				if vid := strings.TrimSpace(fmt.Sprint(data["video_id"])); vid != "" && vid != "<nil>" {
					vItem := map[string]any{"video_id": vid}
					if th, ok := data["image_hash"].(string); ok && strings.TrimSpace(th) != "" {
						vItem["thumbnail_hash"] = th
					} else if tu, ok := data["image_url"].(string); ok && strings.TrimSpace(tu) != "" {
						vItem["thumbnail_url"] = tu
					}
					feed["videos"] = []any{vItem}
				}
			}
			if feed["ad_formats"] == nil && feed["videos"] != nil {
				feed["ad_formats"] = []any{"SINGLE_VIDEO"}
			}
			linkURL, ctaType := extractLinkAndCTA(data)
			if feed["link_urls"] == nil && linkURL != "" {
				feed["link_urls"] = []any{map[string]any{"website_url": linkURL}}
			}
			if feed["call_to_action_types"] == nil && ctaType != "" {
				feed["call_to_action_types"] = []any{ctaType}
			}
			if feed["videos"] != nil && feed["link_urls"] != nil {
				delete(story, "video_data")
			} else {
				delete(data, "message")
				delete(data, "title")
			}
		}
	}
	if len(primaryTexts) > 0 {
		bodies := make([]any, len(primaryTexts))
		for i, s := range primaryTexts {
			bodies[i] = map[string]any{"text": s}
		}
		feed["bodies"] = bodies
	}
	if len(headlines) > 0 {
		titles := make([]any, len(headlines))
		for i, s := range headlines {
			titles[i] = map[string]any{"text": s}
		}
		feed["titles"] = titles
	}
	hasStoryMedia := false
	if story, ok := p["object_story_spec"].(map[string]any); ok {
		hasStoryMedia = story["link_data"] != nil || story["video_data"] != nil
	}
	if !hasStoryMedia && (feed["images"] != nil || feed["videos"] != nil) {
		feed["optimization_type"] = "REGULAR"
	} else if feed["optimization_type"] == nil || feed["optimization_type"] == "" {
		feed["optimization_type"] = "REGULAR"
	}
	p["asset_feed_spec"] = feed
	dof, _ := p["degrees_of_freedom_spec"].(map[string]any)
	if dof == nil {
		dof = map[string]any{}
	}
	cfs, _ := dof["creative_features_spec"].(map[string]any)
	if cfs == nil {
		cfs = map[string]any{}
	}
	if cfs["text_optimizations"] == nil {
		cfs["text_optimizations"] = map[string]any{"enroll_status": "OPT_IN"}
	}
	dof["creative_features_spec"] = cfs
	p["degrees_of_freedom_spec"] = dof
	delete(p, "body")
	delete(p, "title")
	return nil
}

func extractLinkAndCTA(data map[string]any) (string, string) {
	linkURL, _ := data["link"].(string)
	linkURL = strings.TrimSpace(linkURL)
	ctaType := ""
	if cta, ok := data["call_to_action"].(map[string]any); ok {
		if t, ok := cta["type"].(string); ok {
			ctaType = strings.TrimSpace(t)
		}
		if val, ok := cta["value"].(map[string]any); ok && linkURL == "" {
			if l, ok := val["link"].(string); ok {
				linkURL = strings.TrimSpace(l)
			}
		}
	}
	return linkURL, ctaType
}

// CloneCreativeForTextOptions builds a new creative create payload from an
// observed creative object, preserving object_story_spec, url_tags, media/link
// specifications, and any unreplaced text options while applying the new
// primaryTexts and headlines.
func CloneCreativeForTextOptions(observed map[string]any, name string, primaryTexts, headlines []string) (map[string]any, error) {
	if len(primaryTexts) == 0 && len(headlines) == 0 {
		return nil, output.Fail("USAGE", "Provide at least one --primary-text or --headline option.", 2)
	}
	raw, err := json.Marshal(observed)
	if err != nil {
		return nil, output.Fail("VALIDATION", "Cannot serialize existing creative.", 5)
	}
	cloned, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	creativeName := strings.TrimSpace(name)
	if creativeName == "" {
		creativeName, _ = cloned["name"].(string)
		creativeName = strings.TrimSpace(creativeName)
	}
	if creativeName == "" {
		return nil, output.Fail("VALIDATION", "Creative requires a name.", 5)
	}
	story, ok := cloned["object_story_spec"].(map[string]any)
	if !ok || len(story) == 0 {
		return nil, output.Fail("VALIDATION", "Existing creative must have object_story_spec to clone text options.", 5)
	}
	out := map[string]any{
		"name":              creativeName,
		"object_story_spec": story,
	}
	if ut, ok := cloned["url_tags"].(string); ok && strings.TrimSpace(ut) != "" {
		out["url_tags"] = ut
	}
	if afs, ok := cloned["asset_feed_spec"].(map[string]any); ok {
		cleanAFS := map[string]any{}
		if bodies := normalizeTextItems(afs["bodies"]); len(bodies) > 0 {
			cleanAFS["bodies"] = bodies
		}
		if titles := normalizeTextItems(afs["titles"]); len(titles) > 0 {
			cleanAFS["titles"] = titles
		}
		if images := normalizeObjectItems(afs["images"], "hash", "url"); len(images) > 0 {
			cleanAFS["images"] = images
		}
		if videos := normalizeObjectItems(afs["videos"], "video_id", "thumbnail_hash", "thumbnail_url"); len(videos) > 0 {
			cleanAFS["videos"] = videos
		}
		if links := normalizeObjectItems(afs["link_urls"], "website_url", "display_url", "deeplink_url"); len(links) > 0 {
			cleanAFS["link_urls"] = links
		}
		if formats := normalizeStringList(afs["ad_formats"]); len(formats) > 0 {
			cleanAFS["ad_formats"] = formats
		}
		if ctas := normalizeStringList(afs["call_to_action_types"]); len(ctas) > 0 {
			cleanAFS["call_to_action_types"] = ctas
		}
		if opt, ok := afs["optimization_type"].(string); ok && strings.TrimSpace(opt) != "" {
			cleanAFS["optimization_type"] = opt
		}
		if len(cleanAFS) > 0 {
			out["asset_feed_spec"] = cleanAFS
		}
	}
	if err := ApplyTextOptions(out, primaryTexts, headlines); err != nil {
		return nil, err
	}
	if err := Validate("creatives", "create", out); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeTextItems(v any) []any {
	return normalizeObjectItems(v, "text")
}

func normalizeObjectItems(v any, allowedKeys ...string) []any {
	items, ok := v.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		clean := map[string]any{}
		for _, k := range allowedKeys {
			if val := strings.TrimSpace(fmt.Sprint(m[k])); m[k] != nil && val != "" && val != "<nil>" {
				clean[k] = val
			}
		}
		if len(clean) > 0 {
			out = append(out, clean)
		}
	}
	return out
}

func normalizeStringList(v any) []any {
	items, ok := v.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, raw := range items {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
