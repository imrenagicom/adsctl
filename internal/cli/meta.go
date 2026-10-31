package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"github.com/spf13/cobra"
)

type runtime struct {
	Client                    *graph.Client
	Profile, Account, Version string
	Mode                      string
	Ctx                       context.Context
	Cancel                    context.CancelFunc
}

func (a *App) runtime(c *cobra.Command, accountRequired, write bool) (*runtime, error) {
	cfg, _, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	name := a.opts.Profile
	if name == "" {
		name = os.Getenv("ADSCTL_PROFILE")
	}
	if name == "" {
		name = cfg.DefaultProfile
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, output.Fail("CONFIG", "Select a configured Meta profile with --profile.", 2)
	}
	account := a.opts.Account
	if account == "" {
		account = os.Getenv("ADSCTL_ACCOUNT")
	}
	if account != "" {
		account, err = config.Account(account)
		if err != nil {
			return nil, err
		}
	}
	if accountRequired && account == "" {
		return nil, output.Fail("USAGE", "Select an account with --account.", 2)
	}
	if write && (!c.Flags().Changed("profile") || !c.Flags().Changed("account") || p.Mode != "write") {
		return nil, output.Fail("WRITE_SCOPE", "Advertising writes require explicit --profile, --account and a write-mode profile.", 4)
	}
	version := a.opts.APIVersion
	if version == "" {
		version = p.APIVersion
	}
	if !config.ValidVersion(version) {
		return nil, output.Fail("USAGE", "API version must be v<number>.0.", 2)
	}
	token, err := p.Token()
	if err != nil {
		return nil, err
	}
	client := graph.New(token, version)
	client.RequestTimeout = a.opts.RequestTimeout
	if a.baseURL != "" {
		client.BaseURL = a.baseURL
	}
	if a.httpClient != nil {
		client.HTTP = a.httpClient
	}
	if a.opts.Verbose {
		client.Log = a.Err
	}
	if p.AppSecretEnv != "" {
		client.AppSecret = os.Getenv(p.AppSecretEnv)
		if client.AppSecret == "" {
			return nil, output.Fail("CONFIG", "Missing app-secret environment reference.", 2)
		}
	}
	ctx, cancel := context.WithTimeout(c.Context(), a.opts.Timeout)
	a.meta = map[string]any{"platform": "meta", "profile": name, "account_id": account, "api_version": version, "fetched_at": time.Now().UTC().Format(time.RFC3339)}
	return &runtime{Client: client, Profile: name, Account: account, Version: version, Mode: p.Mode, Ctx: ctx, Cancel: cancel}, nil
}
func (a *App) addMeta(root *cobra.Command) {
	meta := group("meta", "Inspect and manage Meta advertising")
	root.AddCommand(meta)
	auth := group("auth", "Check external credentials")
	meta.AddCommand(auth)
	auth.AddCommand(action("check", "Verify access with a bounded account read", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.runtime(c, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		v, err := r.Client.Request(r.Ctx, "GET", r.Account, url.Values{"fields": {"id,name,account_status,currency,timezone_name"}})
		if err != nil {
			return err
		}
		return a.emit(c, map[string]any{"account_access": true, "token_valid_for_request": true, "scopes": "unknown", "mode": r.Mode, "account": v.Value})
	}))
	for _, resource := range []string{"accounts", "campaigns", "adsets", "ads", "creatives", "images", "videos", "pages", "instagram-accounts", "pixels", "audiences", "businesses"} {
		a.addResource(meta, resource)
	}
	targeting := group("targeting", "Search targeting options")
	meta.AddCommand(targeting)
	var typ, query string
	search := action("search", "Find supported targeting IDs", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.runtime(c, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if typ == "" || query == "" {
			return output.Fail("USAGE", "Targeting search requires --type and --query.", 2)
		}
		return a.list(c, r, "search", url.Values{"type": {typ}, "q": {query}}, graph.PageOptions{PageSize: 100, MaxItems: 1000})
	})
	search.Flags().StringVar(&typ, "type", "", "Meta search type, such as adgeolocation")
	search.Flags().StringVar(&query, "query", "", "Search text")
	targeting.AddCommand(search)
	a.addInsights(meta)
	a.addReports(meta)
	a.addInventory(meta)
	a.addMonitor(meta)
	a.addLaunch(meta)
	a.addDiscovery(root, meta)
}

var edges = map[string]string{"campaigns": "campaigns", "adsets": "adsets", "ads": "ads", "creatives": "adcreatives", "images": "adimages", "videos": "advideos", "pixels": "adspixels", "audiences": "customaudiences", "instagram-accounts": "instagram_accounts"}

func (a *App) addResource(meta *cobra.Command, resource string) {
	g := group(resource, "Inspect "+resource)
	meta.AddCommand(g)
	var fields, status, campaign, adset, relationship string
	var all bool
	var cursor string
	var size, maxItems int
	list := action("list", "List "+resource+" with bounded pagination", "read", func(c *cobra.Command, _ []string) error {
		require := resource != "accounts" && resource != "businesses"
		r, err := a.runtime(c, require, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		path := r.Account + "/" + edges[resource]
		switch resource {
		case "accounts":
			path = "me/adaccounts"
		case "businesses":
			path = "me/businesses"
		case "pages":
			path = "me/accounts"
		}
		if campaign != "" && adset != "" {
			return output.Fail("USAGE", "Choose --campaign or --adset, not both.", 2)
		}
		if campaign != "" {
			if err := config.ID(campaign); err != nil {
				return err
			}
			if err := verify(r, "campaigns", campaign); err != nil {
				return err
			}
			path = campaign + "/" + edges[resource]
		}
		if adset != "" {
			if err := config.ID(adset); err != nil {
				return err
			}
			if err := verify(r, "adsets", adset); err != nil {
				return err
			}
			path = adset + "/ads"
		}
		params := url.Values{"fields": {fields}}
		if status != "" {
			b := `["` + strings.Join(strings.Split(status, ","), `","`) + `"]`
			params.Set("effective_status", b)
		}
		return a.list(c, r, path, params, graph.PageOptions{Scope: r.Profile + "|" + r.Account, Cursor: cursor, All: all, PageSize: size, MaxItems: maxItems})
	})
	f := list.Flags()
	f.StringVar(&fields, "fields", defaultFields(resource), "Native fields (Graph expansions accepted)")
	f.BoolVar(&all, "all", false, "Fetch all pages within max-items")
	f.StringVar(&cursor, "cursor", "", "Scoped continuation cursor")
	f.IntVar(&size, "page-size", 100, "Requested page size, 1..1000")
	f.IntVar(&maxItems, "max-items", 10000, "Maximum returned rows")
	if resource == "campaigns" || resource == "adsets" || resource == "ads" {
		f.StringVar(&status, "effective-status", "", "Comma-separated effective statuses")
	}
	if resource == "adsets" || resource == "ads" {
		f.StringVar(&campaign, "campaign", "", "Parent campaign ID")
	}
	if resource == "ads" {
		f.StringVar(&adset, "adset", "", "Parent ad set ID")
	}
	g.AddCommand(list)
	if resource == "images" || resource == "videos" {
		a.addMedia(g, resource)
	}
	if isOwned(resource) {
		a.addMutations(g, resource)
	}
	if resource != "images" {
		var getFields string
		get := action("get ID", "Read "+resource+" by ID", "read", func(c *cobra.Command, args []string) error {
			req := resource != "accounts" && resource != "businesses"
			r, err := a.runtime(c, req, false)
			if err != nil {
				return err
			}
			defer r.Cancel()
			id := args[0]
			if resource == "accounts" {
				id, err = config.Account(id)
			} else {
				err = config.ID(id)
			}
			if err != nil {
				return err
			}
			if resource == "videos" {
				if err = verifyVideo(r, id); err != nil {
					return err
				}
			}
			if resource == "accounts" {
				a.meta["account_id"] = id
			}
			if isOwned(resource) {
				if err = verify(r, resource, id); err != nil {
					return err
				}
			}
			v, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {getFields}})
			if err != nil {
				return err
			}
			return a.emit(c, v.Value)
		})
		get.Args = cobra.ExactArgs(1)
		get.Flags().StringVar(&getFields, "fields", defaultFields(resource), "Native fields")
		g.AddCommand(get)
	}
	if resource == "businesses" {
		ag := group("accounts", "Discover owned or client ad accounts")
		g.AddCommand(ag)
		ac := action("list BUSINESS_ID", "List business ad accounts", "read", func(c *cobra.Command, args []string) error {
			r, err := a.runtime(c, false, false)
			if err != nil {
				return err
			}
			defer r.Cancel()
			if err = config.ID(args[0]); err != nil {
				return err
			}
			if relationship != "owned" && relationship != "client" {
				return output.Fail("USAGE", "Relationship must be owned or client.", 2)
			}
			return a.list(c, r, args[0]+"/"+relationship+"_ad_accounts", url.Values{"fields": {defaultFields("accounts")}}, graph.PageOptions{Scope: r.Profile, All: true, PageSize: 100, MaxItems: 10000})
		})
		ac.Args = cobra.ExactArgs(1)
		ac.Flags().StringVar(&relationship, "relationship", "owned", "owned or client")
		ag.AddCommand(ac)
	}
}
func defaultFields(resource string) string {
	switch resource {
	case "accounts":
		return "id,name,account_status,currency,timezone_name"
	case "campaigns":
		return "id,name,objective,status,effective_status,account_id"
	case "adsets":
		return "id,name,campaign_id,status,effective_status,account_id"
	case "ads":
		return "id,name,campaign_id,adset_id,status,effective_status,account_id,creative"
	case "images":
		return "hash,name,url"
	case "creatives":
		return "id,name,account_id,object_story_spec"
	case "videos":
		return "id,title,status"
	case "audiences":
		return "id,name,subtype,approximate_count_lower_bound,approximate_count_upper_bound,retention_days,operation_status,delivery_status"
	default:
		return "id,name"
	}
}
func isOwned(resource string) bool {
	return resource == "campaigns" || resource == "adsets" || resource == "ads" || resource == "creatives" || resource == "audiences"
}
func verify(r *runtime, resource, id string) error {
	if err := config.ID(id); err != nil {
		return err
	}
	if !isOwned(resource) {
		return nil
	}
	v, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {"id,account_id"}})
	if err != nil {
		return err
	}
	account := strings.TrimPrefix(fmt.Sprint(v.Value["account_id"]), "act_")
	if account != strings.TrimPrefix(r.Account, "act_") {
		return output.Fail("ACCOUNT_MISMATCH", "Resource does not belong to the selected ad account.", 4)
	}
	return nil
}
func (a *App) list(c *cobra.Command, r *runtime, path string, q url.Values, o graph.PageOptions) error {
	if o.Scope == "" {
		o.Scope = r.Profile + "|" + r.Account
	}
	result, err := r.Client.List(r.Ctx, path, q, o)
	a.meta["complete"] = result.Complete
	a.meta["pagination"] = map[string]any{"has_more": !result.Complete, "next_cursor": result.Cursor}
	a.meta["usage"] = result.Usage
	if err != nil {
		if len(result.Rows) > 0 {
			env := output.New(result.Rows, a.meta)
			env.OK = false
			env.Error = output.AsError(err)
			_ = output.Write(a.Out, a.opts.Output, env)
		}
		return err
	}
	return a.emit(c, result.Rows)
}
