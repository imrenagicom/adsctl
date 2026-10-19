// Package launch compiles explicit native payloads into a paused dependency plan.
package launch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"adsctl/internal/document"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/schema"
)

type Manifest struct {
	Version   int     `yaml:"version" json:"version"`
	Kind      string  `yaml:"kind" json:"kind"`
	Name      string  `yaml:"name" json:"name"`
	Assets    []Asset `yaml:"assets" json:"assets"`
	Campaigns []Node  `yaml:"campaigns" json:"campaigns"`
	Adsets    []Node  `yaml:"adsets" json:"adsets"`
	Creatives []Node  `yaml:"creatives" json:"creatives"`
	Ads       []Node  `yaml:"ads" json:"ads"`
}
type Asset struct {
	Key  string `yaml:"key" json:"key"`
	Type string `yaml:"type" json:"type"`
	File string `yaml:"file" json:"file"`
}
type Node struct {
	Key          string         `yaml:"key" json:"key"`
	Payload      map[string]any `yaml:"payload" json:"payload"`
	CampaignRef  string         `yaml:"campaign_ref,omitempty" json:"campaign_ref,omitempty"`
	AdsetRef     string         `yaml:"adset_ref,omitempty" json:"adset_ref,omitempty"`
	CreativeRef  string         `yaml:"creative_ref,omitempty" json:"creative_ref,omitempty"`
	ImageRef     string         `yaml:"image_ref,omitempty" json:"image_ref,omitempty"`
	VideoRef     string         `yaml:"video_ref,omitempty" json:"video_ref,omitempty"`
	PrimaryTexts []string       `yaml:"primary_texts,omitempty" json:"primary_texts,omitempty"`
	Headlines    []string       `yaml:"headlines,omitempty" json:"headlines,omitempty"`
}
type Scope struct {
	Profile    string `json:"profile"`
	Account    string `json:"account_id"`
	APIVersion string `json:"api_version"`
	Currency   string `json:"currency"`
}
type Step struct {
	Key      string `json:"key"`
	Resource string `json:"resource"`
	Node     Node   `json:"node"`
	File     string `json:"file,omitempty"`
	Hash     string `json:"sha256,omitempty"`
}
type Plan struct {
	Version    int    `json:"version"`
	Name       string `json:"name"`
	Scope      Scope  `json:"scope"`
	CreatedAt  string `json:"created_at"`
	Steps      []Step `json:"steps"`
	Digest     string `json:"digest"`
	Validation string `json:"validation"`
}

var keyName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func Compile(b []byte, base string, scope Scope) (Plan, error) {
	var m Manifest
	if err := document.Decode(b, &m); err != nil {
		return Plan{}, err
	}
	if m.Version != 1 || m.Kind != "meta-launch" || !keyName.MatchString(m.Name) {
		return Plan{}, output.Fail("VALIDATION", "Launch requires version 1, kind meta-launch and a valid name.", 5)
	}
	p := Plan{Version: 1, Name: m.Name, Scope: scope, CreatedAt: time.Now().UTC().Format(time.RFC3339), Validation: "Local schema and reference checks; remote conditional validation is deferred to Meta."}
	for _, a := range m.Assets {
		if a.Type != "image" && a.Type != "video" {
			return Plan{}, output.Fail("VALIDATION", "Asset type must be image or video.", 5)
		}
		path := a.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return Plan{}, err
		}
		hash, err := HashFile(path)
		if err != nil {
			return Plan{}, err
		}
		resource := map[string]string{"image": "images", "video": "videos"}[a.Type]
		p.Steps = append(p.Steps, Step{Key: "assets." + a.Key, Resource: resource, Node: Node{Key: a.Key}, File: path, Hash: hash})
	}
	for _, pair := range []struct {
		resource string
		nodes    []Node
	}{{"campaigns", m.Campaigns}, {"adsets", m.Adsets}, {"creatives", m.Creatives}, {"ads", m.Ads}} {
		for _, n := range pair.nodes {
			// Normalize YAML native numbers without passing through float64.
			raw, err := json.Marshal(n.Payload)
			if err != nil {
				return Plan{}, output.Fail("VALIDATION", "Payload must use JSON-compatible native fields.", 5)
			}
			n.Payload, err = schema.Decode(raw)
			if err != nil {
				return Plan{}, err
			}
			if len(n.PrimaryTexts) > 0 || len(n.Headlines) > 0 {
				if pair.resource != "creatives" {
					return Plan{}, output.Fail("VALIDATION", "primary_texts and headlines are only valid on creatives.", 5)
				}
				if err := schema.ApplyTextOptions(n.Payload, n.PrimaryTexts, n.Headlines); err != nil {
					return Plan{}, err
				}
			}
			schema.Defaults(pair.resource, n.Payload)
			p.Steps = append(p.Steps, Step{Key: pair.resource + "." + n.Key, Resource: pair.resource, Node: n})
		}
	}
	if err := p.Validate(false); err != nil {
		return Plan{}, err
	}
	p.Digest = p.Identity()
	return p, nil
}
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", output.Fail("INPUT", "Cannot open referenced media file.", 2)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<30 {
		return "", output.Fail("INPUT", "Media must be a nonempty regular file at most 1 GiB.", 2)
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (p Plan) Identity() string {
	p.Digest = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (p Plan) Validate(checkDigest bool) error {
	if p.Version != 1 || !keyName.MatchString(p.Name) || len(p.Steps) == 0 || len(p.Steps) > 1000 {
		return output.Fail("VALIDATION", "Invalid plan schema, name or step count.", 5)
	}
	if checkDigest && p.Digest != p.Identity() {
		return output.Fail("PLAN_CHANGED", "Plan identity changed; generate a new plan.", 5)
	}
	known := map[string]string{}
	ids := map[string]string{}
	order := map[string]int{"images": 0, "videos": 0, "campaigns": 1, "adsets": 2, "creatives": 3, "ads": 4}
	last := 0
	for _, s := range p.Steps {
		priority, ok := order[s.Resource]
		prefix := s.Resource
		if s.Resource == "images" || s.Resource == "videos" {
			prefix = "assets"
		}
		if !ok || priority < last || !keyName.MatchString(s.Node.Key) || s.Key != prefix+"."+s.Node.Key || known[s.Key] != "" {
			return output.Fail("VALIDATION", "Plan requires unique keys and topologically ordered supported resources.", 5)
		}
		last = priority
		for _, ref := range References(s.Node) {
			if known[ref] == "" {
				return output.Fail("VALIDATION", "Missing or forward reference: "+ref, 5)
			}
		}
		if s.Node.ImageRef != "" && known["assets."+s.Node.ImageRef] != "images" {
			return output.Fail("VALIDATION", "image_ref must name an image.", 5)
		}
		if s.Node.VideoRef != "" && known["assets."+s.Node.VideoRef] != "videos" {
			return output.Fail("VALIDATION", "video_ref must name a video.", 5)
		}
		if s.Resource == "images" || s.Resource == "videos" {
			if s.File == "" || !filepath.IsAbs(s.File) || len(s.Hash) != 64 || s.Node.Payload != nil || len(References(s.Node)) != 0 {
				return output.Fail("VALIDATION", "Invalid media plan step.", 5)
			}
		} else {
			if s.File != "" || s.Hash != "" {
				return output.Fail("VALIDATION", "Only asset steps may reference files.", 5)
			}
			if s.Resource != "creatives" && s.Node.Payload["status"] != "PAUSED" {
				return output.Fail("VALIDATION", "Launch creates must remain PAUSED; activate the run separately.", 5)
			}
			if s.Node.CampaignRef != "" && s.Resource != "adsets" || s.Node.AdsetRef != "" && s.Resource != "ads" || s.Node.CreativeRef != "" && s.Resource != "ads" || (s.Node.ImageRef != "" || s.Node.VideoRef != "" || len(s.Node.PrimaryTexts) > 0 || len(s.Node.Headlines) > 0) && s.Resource != "creatives" {
				return output.Fail("VALIDATION", "Reference is not valid for this resource.", 5)
			}
			payload, err := Payload(s, ids)
			if err != nil {
				return err
			}
			if err = schema.Validate(s.Resource, "create", payload); err != nil {
				return err
			}
		}
		known[s.Key] = s.Resource
		ids[s.Key] = "1"
	}
	return nil
}
func References(n Node) []string {
	out := []string{}
	for _, p := range []struct{ prefix, value string }{{"campaigns", n.CampaignRef}, {"adsets", n.AdsetRef}, {"creatives", n.CreativeRef}, {"assets", n.ImageRef}, {"assets", n.VideoRef}} {
		if p.value != "" {
			out = append(out, p.prefix+"."+p.value)
		}
	}
	return out
}
func Payload(s Step, ids map[string]string) (map[string]any, error) {
	raw, err := json.Marshal(s.Node.Payload)
	if err != nil {
		return nil, err
	}
	p, err := schema.Decode(raw)
	if err != nil {
		return nil, err
	}
	n := s.Node
	if len(n.PrimaryTexts) > 0 || len(n.Headlines) > 0 {
		if err := schema.ApplyTextOptions(p, n.PrimaryTexts, n.Headlines); err != nil {
			return nil, err
		}
	}
	for _, ref := range []struct{ key, field, value string }{{"campaigns", "campaign_id", n.CampaignRef}, {"adsets", "adset_id", n.AdsetRef}, {"creatives", "creative", n.CreativeRef}} {
		if ref.value != "" {
			if p[ref.field] != nil {
				return nil, output.Fail("VALIDATION", "Reference conflicts with native "+ref.field, 5)
			}
			id := ids[ref.key+"."+ref.value]
			if id == "" {
				return nil, output.Fail("DEPENDENCY", "Dependency is not yet available.", 9)
			}
			if ref.field == "creative" {
				p[ref.field] = map[string]any{"creative_id": id}
			} else {
				p[ref.field] = id
			}
		}
	}
	if n.ImageRef != "" && n.VideoRef != "" {
		return nil, output.Fail("VALIDATION", "Choose image_ref or video_ref.", 5)
	}
	if n.ImageRef != "" || n.VideoRef != "" {
		story, ok := p["object_story_spec"].(map[string]any)
		if !ok {
			return nil, output.Fail("VALIDATION", "Asset references require object_story_spec.", 5)
		}
		dataKey, field, ref := "link_data", "image_hash", n.ImageRef
		if n.VideoRef != "" {
			dataKey, field, ref = "video_data", "video_id", n.VideoRef
		}
		data, ok := story[dataKey].(map[string]any)
		if !ok {
			return nil, output.Fail("VALIDATION", "Asset reference requires object_story_spec."+dataKey, 5)
		}
		if data[field] != nil {
			return nil, output.Fail("VALIDATION", "Asset reference conflicts with native "+field, 5)
		}
		id := ids["assets."+ref]
		if id == "" {
			return nil, output.Fail("DEPENDENCY", "Media dependency is unavailable.", 9)
		}
		data[field] = id
	}
	return p, nil
}
func (p Plan) CheckFiles() error {
	for _, s := range p.Steps {
		if s.File != "" {
			hash, err := HashFile(s.File)
			if err != nil {
				return err
			}
			if hash != s.Hash {
				return output.Fail("ASSET_CHANGED", "Referenced media changed: "+filepath.Base(s.File), 5)
			}
		}
	}
	return nil
}
func (p Plan) Step(key string) (Step, bool) {
	for _, s := range p.Steps {
		if s.Key == key {
			return s, true
		}
	}
	return Step{}, false
}
func (p Plan) String() string {
	return fmt.Sprintf("%s (%d steps, %s)", p.Name, len(p.Steps), strings.TrimPrefix(p.Scope.Account, "act_"))
}
