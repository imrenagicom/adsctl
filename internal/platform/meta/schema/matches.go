package schema

import (
	"fmt"
	"strings"
	"time"
)

// MatchesPayload reports whether every planned field matches the observed
// native read. Provider-normalized set fields are compared without regard to
// element order.
func MatchesPayload(expected, observed map[string]any) bool {
	return FirstMismatch("", expected, observed) == ""
}

// FirstMismatch returns the dotted path of the first planned field that does
// not match the observed native read, or "" when every planned field matches.
// A path (rather than a bare bool) lets callers report which field drifted.
//
// resource scopes provider normalization: Meta may append a suffix to a
// creative's name after create, so creative names compare by prefix. Set-valued
// fields (for example placement lists under targeting) compare by membership;
// every other array stays positional.
func FirstMismatch(resource string, expected, observed map[string]any) string {
	return diffPayload(resource, "", expected, observed)
}

func diffPayload(resource, path string, expected, observed map[string]any) string {
	for k, v := range expected {
		field := joinPath(path, k)
		actual, ok := observed[k]
		if k == "creative" {
			if want, wok := v.(map[string]any); wok {
				if got, gok := actual.(map[string]any); gok {
					id := got["id"]
					if id == nil {
						id = got["creative_id"]
					}
					if fmt.Sprint(want["creative_id"]) != fmt.Sprint(id) {
						return field
					}
					continue
				}
			}
		}
		if !ok && resource == "creatives" {
			switch k {
			case "object_id":
				actual, ok = observed["actor_id"]
				if !ok {
					if story, sok := observed["object_story_spec"].(map[string]any); sok {
						actual, ok = story["page_id"]
					}
				}
			case "source_instagram_media_id":
				actual, ok = observed["effective_instagram_media_id"]
			}
		}
		if !ok {
			return field
		}
		if k == "name" && resource == "creatives" {
			if want, wok := v.(string); wok {
				if got, gok := actual.(string); gok && nameNormalized(want, got) {
					continue
				}
			}
		}
		if mismatch := matchesValue(resource, field, v, actual); mismatch != "" {
			return mismatch
		}
	}
	return ""
}

func matchesValue(resource, path string, want, got any) string {
	switch v := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return path
		}
		return diffPayload(resource, path, v, g)
	case []any:
		g, ok := got.([]any)
		if ok && path == "targeting.geo_locations.location_types" {
			if locationTypesMatch(v, g) {
				return ""
			}
			return path
		}
		if !ok || len(v) != len(g) {
			return path
		}
		if isSetValued(path) {
			if matchesSet(resource, path, v, g) {
				return ""
			}
			return path
		}
		for i := range v {
			if mismatch := matchesValue(resource, fmt.Sprintf("%s[%d]", path, i), v[i], g[i]); mismatch != "" {
				return mismatch
			}
		}
		return ""
	default:
		if isTimeField(path) && sameInstant(want, got) {
			return ""
		}
		if fmt.Sprint(want) == fmt.Sprint(got) {
			return ""
		}
		return path
	}
}

// matchesSet compares two arrays as unordered sets. Duplicates in the planned
// value must each be matched by a distinct observed element.
func matchesSet(resource, path string, want, got []any) bool {
	used := make([]bool, len(got))
	for _, w := range want {
		found := false
		for i, g := range got {
			if used[i] {
				continue
			}
			if matchesValue(resource, path, w, g) == "" {
				used[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// locationTypesMatch compares geo location types as a set. Meta expands a
// request for both "home" and "recent" to also include "frequently_in", so
// that single provider-added value is accepted; any other difference fails.
func locationTypesMatch(want, got []any) bool {
	planned := map[string]bool{}
	for _, w := range want {
		planned[fmt.Sprint(w)] = true
	}
	observed := map[string]bool{}
	for _, g := range got {
		observed[fmt.Sprint(g)] = true
	}
	for k := range planned {
		if !observed[k] {
			return false
		}
	}
	expanded := planned["home"] && planned["recent"]
	for k := range observed {
		if planned[k] || (k == "frequently_in" && expanded) {
			continue
		}
		return false
	}
	return true
}

// nameNormalized reports whether an observed provider name is the planned name
// with an appended provider marker. Meta appends a date/hash suffix to some
// creative names, so a leading match followed by a separator is treated as the
// same name; a wholly different name still fails.
func nameNormalized(planned, observed string) bool {
	if planned == observed {
		return true
	}
	rest, ok := strings.CutPrefix(observed, planned)
	if !ok || rest == "" {
		return false
	}
	switch rest[0] {
	case ' ', '-', '_', '(', '|':
		return true
	default:
		return false
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// setValuedFields lists native fields Meta treats as unordered sets. The
// provider may return these in a normalized order that differs from the planned
// order, so comparison checks membership instead of position. The registry is
// intentionally narrow: only add fields the provider documents as sets, and
// keep strictly ordered arrays positional.
var setValuedFields = map[string]bool{
	"asset_feed_spec.ad_formats":           true,
	"asset_feed_spec.bodies":               true,
	"asset_feed_spec.call_to_action_types": true,
	"asset_feed_spec.titles":               true,
	"special_ad_categories":                true,
	"targeting.custom_audiences":           true,
	"targeting.excluded_custom_audiences":  true,
	"targeting.publisher_platforms":        true,
	"targeting.facebook_positions":         true,
	"targeting.instagram_positions":        true,
	"targeting.messenger_positions":        true,
	"targeting.audience_network_positions": true,
	"targeting.device_platforms":           true,
}

func isSetValued(path string) bool {
	return setValuedFields[path]
}

// timeFields lists native timestamp fields where Meta may return a normalized
// UTC offset format (such as "+0700" instead of RFC 3339 "+07:00" or "Z").
var timeFields = map[string]bool{
	"start_time":             true,
	"end_time":               true,
	"stop_time":              true,
	"created_time":           true,
	"updated_time":           true,
	"ad_schedule_start_time": true,
	"ad_schedule_end_time":   true,
}

var timeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05.999999999-0700",
}

func isTimeField(path string) bool {
	return timeFields[path]
}

func sameInstant(want, got any) bool {
	wt, wok := parseInstant(want)
	gt, gok := parseInstant(got)
	return wok && gok && wt.Equal(gt)
}

func parseInstant(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
