package cli

import (
	"fmt"
	"net/url"
	"strings"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"adsctl/internal/platform/meta/schema"
	"github.com/spf13/cobra"
)

type mutationFlags struct {
	File, Name, Objective, Categories, Adset, Creative, Status string
	DailyBudget, LifetimeBudget, BidStrategy                   string
	AdvantageBudget, AdvantagePlacements                       bool
	PrimaryTexts, Headlines                                    []string
	Dry                                                        bool
}

func (a *App) addMutations(g *cobra.Command, resource string) {
	for _, op := range []string{"create", "update", "validate", "pause", "activate", "archive", "delete"} {
		if (resource == "creatives" || resource == "audiences") && (op == "pause" || op == "activate" || op == "archive") {
			continue
		}
		f := &mutationFlags{}
		operation, id := "create", ""
		use := op
		if op != "create" && op != "validate" {
			use += " ID"
		}
		effect := "write"
		if op == "validate" {
			effect = "read"
		}
		cmd := action(use, op+" "+resource+" with explicit account scope", effect, nil)
		if op != "create" && op != "validate" {
			cmd.Args = cobra.ExactArgs(1)
		}
		cmd.Flags().BoolVar(&f.Dry, "dry-run", false, "Return plan without advertising writes")
		if op == "create" || op == "update" || op == "validate" {
			cmd.Flags().StringVar(&f.File, "file", "", "Native JSON payload path or - for stdin")
			if resource == "campaigns" {
				cmd.Flags().BoolVar(&f.AdvantageBudget, "advantage-budget", false, "Enable Advantage+ campaign budget (defaults bid_strategy to LOWEST_COST_WITHOUT_CAP)")
				cmd.Flags().StringVar(&f.DailyBudget, "daily-budget-minor", "", "Campaign daily budget in native minor units")
				cmd.Flags().StringVar(&f.LifetimeBudget, "lifetime-budget-minor", "", "Campaign lifetime budget in native minor units")
				cmd.Flags().StringVar(&f.BidStrategy, "bid-strategy", "", "Native campaign bid strategy")
			}
			if resource == "adsets" {
				cmd.Flags().BoolVar(&f.AdvantagePlacements, "advantage-placements", false, "Enable Advantage+ placements by omitting manual platform/position restrictions from targeting")
			}
			if resource == "creatives" && (op == "create" || op == "validate") {
				cmd.Flags().StringArrayVar(&f.PrimaryTexts, "primary-text", nil, "Primary text option (repeatable, 1 to 5 options)")
				cmd.Flags().StringArrayVar(&f.Headlines, "headline", nil, "Headline option (repeatable, 1 to 5 options)")
			}
			if op != "validate" {
				cmd.Flags().StringVar(&f.Name, "name", "", "Native name")
				if resource != "creatives" && resource != "audiences" {
					cmd.Flags().StringVar(&f.Status, "status", "", "Native status (create defaults PAUSED)")
				}
				if resource == "campaigns" {
					cmd.Flags().StringVar(&f.Objective, "objective", "", "Native campaign objective")
					cmd.Flags().StringVar(&f.Categories, "special-ad-categories", "", "Comma-separated categories or NONE")
				}
				if resource == "ads" {
					cmd.Flags().StringVar(&f.Creative, "creative", "", "Existing creative ID")
					if op == "create" {
						cmd.Flags().StringVar(&f.Adset, "adset", "", "Existing ad set ID")
					}
				}
			} else {
				cmd.Flags().StringVar(&operation, "operation", "create", "create or update")
				cmd.Flags().StringVar(&id, "id", "", "Existing object ID for update validation")
			}
		}
		cmd.RunE = func(c *cobra.Command, args []string) error {
			actual := op
			objID := ""
			if len(args) > 0 {
				objID = args[0]
			}
			if op == "validate" {
				actual = operation
				objID = id
			}
			write := op != "validate"
			r, err := a.runtime(c, true, write)
			if err != nil {
				return err
			}
			defer r.Cancel()
			var payload map[string]any
			switch actual {
			case "create", "update":
				payload, err = a.payload(c, resource, actual, f)
			case "pause":
				payload = map[string]any{"status": "PAUSED"}
			case "activate":
				payload = map[string]any{"status": "ACTIVE"}
			case "archive":
				payload = map[string]any{"status": "ARCHIVED"}
			case "delete":
				payload = map[string]any{}
			default:
				return output.Fail("USAGE", "Operation must be create or update.", 2)
			}
			if err != nil {
				return err
			}
			if actual != "create" {
				if err = verify(r, resource, objID); err != nil {
					return err
				}
			}
			if err = verifyParents(r, resource, payload); err != nil {
				return err
			}
			if op == "validate" {
				return a.emit(c, map[string]any{"local_valid": true, "ownership_checked": true, "remote_validation": "not performed; conditional requirements are validated by Meta on mutation", "payload": payload})
			}
			receipt, err := mutate(r, resource, actual, objID, payload, f.Dry)
			if err != nil {
				return a.partial(c, receipt, err)
			}
			return a.emit(c, receipt)
		}
		g.AddCommand(cmd)
	}
	if resource == "campaigns" || resource == "adsets" {
		a.addBudget(g, resource)
	}
	if resource == "adsets" {
		a.addAdsetPlacements(g)
	}
	if resource == "ads" {
		g2 := group("creative", "Change an ad's creative reference")
		g.AddCommand(g2)
		var creative string
		var dry bool
		cmd := action("set ID", "Reference an existing creative", "write", func(c *cobra.Command, args []string) error {
			r, err := a.runtime(c, true, true)
			if err != nil {
				return err
			}
			defer r.Cancel()
			if err = verify(r, "ads", args[0]); err != nil {
				return err
			}
			if err = verify(r, "creatives", creative); err != nil {
				return err
			}
			receipt, err := mutate(r, "ads", "update", args[0], map[string]any{"creative": map[string]any{"creative_id": creative}}, dry)
			if err != nil {
				return a.partial(c, receipt, err)
			}
			return a.emit(c, receipt)
		})
		cmd.Args = cobra.ExactArgs(1)
		cmd.Flags().StringVar(&creative, "creative", "", "Creative ID")
		cmd.Flags().BoolVar(&dry, "dry-run", false, "Plan without writing")
		g2.AddCommand(cmd)
		a.addAdText(g)
	}
	if resource == "ads" || resource == "creatives" {
		a.addPreview(g, resource)
	}
}
func (a *App) payload(c *cobra.Command, resource, op string, f *mutationFlags) (map[string]any, error) {
	p := map[string]any{}
	if f.File != "" {
		for _, flag := range []string{"name", "objective", "special-ad-categories", "adset", "creative", "status"} {
			if c.Flags().Changed(flag) {
				return nil, output.Fail("USAGE", "File payload cannot be combined with scalar write flags.", 2)
			}
		}
		b, err := a.readFile(f.File)
		if err != nil {
			return nil, err
		}
		p, err = schema.Decode(b)
		if err != nil {
			return nil, err
		}
	} else {
		if f.Name != "" {
			p["name"] = f.Name
		}
		if f.Objective != "" {
			p["objective"] = f.Objective
		}
		if f.Status != "" {
			p["status"] = f.Status
		}
		if f.Categories != "" {
			cats := []any{}
			if f.Categories != "NONE" {
				for _, s := range strings.Split(f.Categories, ",") {
					cats = append(cats, s)
				}
			}
			p["special_ad_categories"] = cats
		}
		if f.Adset != "" {
			p["adset_id"] = f.Adset
		}
		if f.Creative != "" {
			p["creative"] = map[string]any{"creative_id": f.Creative}
		}
		if f.DailyBudget != "" {
			p["daily_budget"] = f.DailyBudget
		}
		if f.LifetimeBudget != "" {
			p["lifetime_budget"] = f.LifetimeBudget
		}
		if f.BidStrategy != "" {
			p["bid_strategy"] = f.BidStrategy
		}
	}
	if f.AdvantageBudget || (f.File != "" && (f.DailyBudget != "" || f.LifetimeBudget != "" || f.BidStrategy != "")) {
		if resource != "campaigns" {
			return nil, output.Fail("USAGE", "Advantage+ budget flags are only supported on campaigns.", 2)
		}
		if err := schema.ApplyAdvantageBudget(p, f.DailyBudget, f.LifetimeBudget, f.BidStrategy); err != nil {
			return nil, err
		}
	}
	if f.AdvantagePlacements {
		if resource != "adsets" {
			return nil, output.Fail("USAGE", "--advantage-placements is only supported on adsets.", 2)
		}
		if err := schema.ApplyAdvantagePlacements(p); err != nil {
			return nil, err
		}
	}
	if len(f.PrimaryTexts) > 0 || len(f.Headlines) > 0 {
		if op != "create" || resource != "creatives" {
			return nil, output.Fail("USAGE", "Text option flags are only supported when creating or validating creatives.", 2)
		}
		if err := schema.ApplyTextOptions(p, f.PrimaryTexts, f.Headlines); err != nil {
			return nil, err
		}
	}
	if op == "create" {
		schema.Defaults(resource, p)
	}
	return p, schema.Validate(resource, op, p)
}
func verifyParents(r *runtime, resource string, p map[string]any) error {
	for field, parent := range map[string]string{"campaign_id": "campaigns", "adset_id": "adsets"} {
		if id, ok := p[field]; ok {
			if err := verify(r, parent, fmt.Sprint(id)); err != nil {
				return err
			}
		}
	}
	if obj, ok := p["creative"].(map[string]any); ok {
		return verify(r, "creatives", fmt.Sprint(obj["creative_id"]))
	}
	return nil
}
func mutate(r *runtime, resource, op, id string, p map[string]any, dry bool) (map[string]any, error) {
	method, path := "POST", id
	if op == "create" {
		path = r.Account + "/" + edges[resource]
	}
	if op == "delete" {
		method = "DELETE"
	}
	receipt := map[string]any{"operation": op, "resource": resource, "id": id, "payload": p, "dry_run": dry, "state": "planned"}
	if dry {
		return receipt, nil
	}
	params, err := graph.Form(p)
	if err != nil {
		return nil, err
	}
	v, err := r.Client.Request(r.Ctx, method, path, params)
	if err != nil {
		receipt["state"] = "failed"
		if output.AsError(err).Exit == 10 {
			receipt["state"] = "unknown"
		}
		return receipt, err
	}
	receipt["response"] = v.Value
	if op == "create" {
		id = fmt.Sprint(v.Value["id"])
		if config.ID(id) != nil {
			receipt["state"] = "unknown"
			return receipt, output.Fail("MUTATION_UNKNOWN", "Create returned no valid ID; reconcile before retrying.", 10)
		}
		receipt["id"] = id
	} else if v.Value["success"] != true {
		receipt["state"] = "unknown"
		return receipt, output.Fail("MUTATION_UNKNOWN", "Mutation did not acknowledge success; reconcile before retrying.", 10)
	}
	receipt["state"] = "applied"
	if op == "delete" {
		receipt["verification"] = "provider acknowledged deletion"
		return receipt, nil
	}
	fields := schema.Keys(p)
	switch resource {
	case "creatives":
	case "audiences":
		fields = joinFields(fields, "approximate_count_lower_bound,approximate_count_upper_bound,operation_status,delivery_status")
	default:
		fields = joinFields(fields, "status,effective_status")
	}
	observed, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {fields}})
	if err != nil {
		cause := output.AsError(err)
		receipt["state"] = "applied_unverified"
		receipt["verification_error"] = cause
		outErr := output.Fail("APPLIED_UNVERIFIED", "Write acknowledged but verification read failed; do not recreate.", 9)
		outErr.HTTPStatus = cause.HTTPStatus
		outErr.Provider = cause.Provider
		return receipt, outErr
	}
	receipt["observed"] = observed.Value
	if fmt.Sprint(observed.Value["account_id"]) != strings.TrimPrefix(r.Account, "act_") {
		receipt["state"] = "applied_unverified"
		return receipt, output.Fail("APPLIED_UNVERIFIED", "Could not confirm resource account after write.", 9)
	}
	if status, ok := p["status"]; ok && observed.Value["status"] != status {
		receipt["state"] = "applied_unverified"
		return receipt, output.Fail("APPLIED_UNVERIFIED", "Configured status has not converged to the requested value.", 9)
	}
	if path := schema.FirstMismatch(resource, schema.Comparable(resource, p), observed.Value); path != "" {
		receipt["state"] = "applied_unverified"
		return receipt, output.Fail("APPLIED_UNVERIFIED", "Observed field "+path+" has not converged to the requested value; do not recreate.", 9)
	}
	receipt["state"] = "verified"
	receipt["verification"] = "read after write; observed fields returned for comparison"
	return receipt, nil
}
func (a *App) addBudget(g *cobra.Command, resource string) {
	bg := group("budget", "Set budget without changing ownership")
	g.AddCommand(bg)
	var daily, lifetime, currency string
	var dry bool
	cmd := action("set ID", "Set exactly one native minor-unit budget", "write", func(c *cobra.Command, args []string) error {
		if (daily == "") == (lifetime == "") || currency == "" {
			return output.Fail("USAGE", "Require one budget and explicit --currency.", 2)
		}
		r, err := a.runtime(c, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = verify(r, resource, args[0]); err != nil {
			return err
		}
		account, err := r.Client.Request(r.Ctx, "GET", r.Account, url.Values{"fields": {"id,currency"}})
		if err != nil {
			return err
		}
		if account.Value["currency"] != currency {
			return output.Fail("CURRENCY_MISMATCH", "Budget currency differs from account currency.", 5)
		}
		obj, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {"id,daily_budget,lifetime_budget" + map[bool]string{true: ",campaign_id"}[resource == "adsets"]}})
		if err != nil {
			return err
		}
		if resource == "adsets" {
			parent := fmt.Sprint(obj.Value["campaign_id"])
			if err = verify(r, "campaigns", parent); err != nil {
				return err
			}
			campaign, err := r.Client.Request(r.Ctx, "GET", parent, url.Values{"fields": {"id,daily_budget,lifetime_budget"}})
			if err != nil {
				return err
			}
			if positive(campaign.Value["daily_budget"]) || positive(campaign.Value["lifetime_budget"]) {
				return output.Fail("BUDGET_OWNERSHIP", "Campaign owns the budget; change the campaign instead.", 5)
			}
		} else if !positive(obj.Value["daily_budget"]) && !positive(obj.Value["lifetime_budget"]) {
			return output.Fail("BUDGET_OWNERSHIP", "Campaign does not currently own a budget.", 5)
		}
		field, value := "daily_budget", daily
		other := "lifetime_budget"
		if lifetime != "" {
			field, value = "lifetime_budget", lifetime
			other = "daily_budget"
		}
		if positive(obj.Value[other]) {
			return output.Fail("BUDGET_TYPE", "Changing budget type requires a reviewed native update.", 5)
		}
		p := map[string]any{field: value}
		if err = schema.Validate(resource, "update", p); err != nil {
			return err
		}
		if !positive(value) {
			return output.Fail("VALIDATION", "Budget must be positive.", 5)
		}
		receipt, err := mutate(r, resource, "update", args[0], p, dry)
		if err != nil {
			return a.partial(c, receipt, err)
		}
		return a.emit(c, receipt)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&daily, "daily-budget-minor", "", "Daily budget in native minor units")
	cmd.Flags().StringVar(&lifetime, "lifetime-budget-minor", "", "Lifetime budget in native minor units")
	cmd.Flags().StringVar(&currency, "currency", "", "Expected account currency")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Plan without writing")
	bg.AddCommand(cmd)
}
func positive(v any) bool { s := fmt.Sprint(v); return s != "" && s != "0" && s != "<nil>" }
func (a *App) addPreview(g *cobra.Command, resource string) {
	var format string
	cmd := action("preview ID", "Return provider preview HTML without executing it", "read", func(c *cobra.Command, args []string) error {
		r, err := a.runtime(c, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = verify(r, resource, args[0]); err != nil {
			return err
		}
		return a.list(c, r, args[0]+"/previews", url.Values{"ad_format": {format}}, graph.PageOptions{All: true, PageSize: 100, MaxItems: 100})
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringVar(&format, "format", "DESKTOP_FEED_STANDARD", "Native ad preview format")
	g.AddCommand(cmd)
}
func (a *App) addAdText(g *cobra.Command) {
	tg := group("text", "Update primary text and headline options by creating and attaching a new creative")
	g.AddCommand(tg)
	var primaryTexts, headlines []string
	var name string
	var dry bool
	cmd := action("set ID", "Set 1 to 5 primary text and/or headline options on an ad", "write", func(c *cobra.Command, args []string) error {
		if len(primaryTexts) == 0 && len(headlines) == 0 {
			return output.Fail("USAGE", "Provide at least one --primary-text or --headline option.", 2)
		}
		r, err := a.runtime(c, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = verify(r, "ads", args[0]); err != nil {
			return err
		}
		adObj, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {"id,name,creative{id}"}})
		if err != nil {
			return err
		}
		creativeMap, _ := adObj.Value["creative"].(map[string]any)
		creativeID := strings.TrimSpace(fmt.Sprint(creativeMap["id"]))
		if creativeID == "" || creativeID == "<nil>" {
			return output.Fail("VALIDATION", "Ad does not have a readable creative ID.", 5)
		}
		if err = verify(r, "creatives", creativeID); err != nil {
			return err
		}
		creativeObj, err := r.Client.Request(r.Ctx, "GET", creativeID, url.Values{"fields": {"id,name,object_story_spec,asset_feed_spec,url_tags"}})
		if err != nil {
			return err
		}
		creativePayload, err := schema.CloneCreativeForTextOptions(creativeObj.Value, name, primaryTexts, headlines)
		if err != nil {
			return err
		}
		if dry {
			return a.emit(c, map[string]any{
				"resource":             "ads",
				"operation":            "text.set",
				"id":                   args[0],
				"previous_creative_id": creativeID,
				"creative_payload":     creativePayload,
				"state":                "dry_run",
			})
		}
		creativeReceipt, err := mutate(r, "creatives", "create", "", creativePayload, false)
		if err != nil {
			return a.partial(c, map[string]any{
				"resource":             "ads",
				"operation":            "text.set",
				"id":                   args[0],
				"previous_creative_id": creativeID,
				"creative":             creativeReceipt,
				"state":                "partial",
			}, err)
		}
		newCreativeID := fmt.Sprint(creativeReceipt["id"])
		adPayload := map[string]any{"creative": map[string]any{"creative_id": newCreativeID}}
		adReceipt, err := mutate(r, "ads", "update", args[0], adPayload, false)
		adReceipt["operation"] = "text.set"
		adReceipt["previous_creative_id"] = creativeID
		adReceipt["creative_id"] = newCreativeID
		adReceipt["creative"] = creativeReceipt
		if err != nil {
			return a.partial(c, adReceipt, err)
		}
		return a.emit(c, adReceipt)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringArrayVar(&primaryTexts, "primary-text", nil, "Primary text option (repeatable, 1 to 5 options)")
	cmd.Flags().StringArrayVar(&headlines, "headline", nil, "Headline option (repeatable, 1 to 5 options)")
	cmd.Flags().StringVar(&name, "name", "", "Optional name for the newly created creative")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Plan without writing")
	tg.AddCommand(cmd)
}

func (a *App) addAdsetPlacements(g *cobra.Command) {
	pg := group("placements", "Configure Advantage+ or manual placements while preserving audience targeting")
	g.AddCommand(pg)
	var advantagePlus, dry bool
	var pubs, igPos, fbPos, msgPos, anPos, softOptOut string
	cmd := action("set ID", "Switch an ad set between Advantage+ placements and manual placements without losing targeting", "write", func(c *cobra.Command, args []string) error {
		r, err := a.runtime(c, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = verify(r, "adsets", args[0]); err != nil {
			return err
		}
		adsetObj, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {"id,name,targeting"}})
		if err != nil {
			return err
		}
		observedTargeting, _ := adsetObj.Value["targeting"].(map[string]any)
		manual := map[string][]string{}
		for key, raw := range map[string]string{
			"publisher_platforms":        pubs,
			"instagram_positions":        igPos,
			"facebook_positions":         fbPos,
			"messenger_positions":        msgPos,
			"audience_network_positions": anPos,
		} {
			if strings.TrimSpace(raw) != "" {
				manual[key] = strings.Split(raw, ",")
			}
		}
		newTargeting, err := schema.CloneTargetingForPlacements(observedTargeting, advantagePlus, manual)
		if err != nil {
			return err
		}
		payload := map[string]any{"targeting": newTargeting}
		if strings.TrimSpace(softOptOut) != "" {
			switch strings.ToLower(strings.TrimSpace(softOptOut)) {
			case "true":
				payload["placement_soft_opt_out"] = true
			case "false":
				payload["placement_soft_opt_out"] = false
			default:
				return output.Fail("USAGE", "--placement-soft-opt-out must be true or false.", 2)
			}
		}
		if err = schema.Validate("adsets", "update", payload); err != nil {
			return err
		}
		receipt, err := mutate(r, "adsets", "update", args[0], payload, dry)
		receipt["operation"] = "placements.set"
		if advantagePlus {
			receipt["placements_mode"] = "advantage_plus"
		} else {
			receipt["placements_mode"] = "manual"
		}
		if err != nil {
			return a.partial(c, receipt, err)
		}
		if !dry && advantagePlus {
			if obs, ok := receipt["observed"].(map[string]any); ok {
				if obsTarg, ok := obs["targeting"].(map[string]any); ok {
					for _, k := range schema.ManualPlacementKeys {
						if obsTarg[k] != nil {
							receipt["state"] = "applied_unverified"
							return a.partial(c, receipt, output.Fail("APPLIED_UNVERIFIED", "Observed targeting."+k+" was not cleared for Advantage+ placements.", 9))
						}
					}
				}
			}
		}
		return a.emit(c, receipt)
	})
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().BoolVar(&advantagePlus, "advantage-plus", false, "Enable Advantage+ placements across all eligible platforms")
	cmd.Flags().StringVar(&pubs, "publisher-platforms", "", "Comma-separated manual publisher platforms (for example instagram,facebook)")
	cmd.Flags().StringVar(&igPos, "instagram-positions", "", "Comma-separated manual Instagram positions (for example stream,story,reels)")
	cmd.Flags().StringVar(&fbPos, "facebook-positions", "", "Comma-separated manual Facebook positions")
	cmd.Flags().StringVar(&msgPos, "messenger-positions", "", "Comma-separated manual Messenger positions")
	cmd.Flags().StringVar(&anPos, "audience-network-positions", "", "Comma-separated manual Audience Network positions")
	cmd.Flags().StringVar(&softOptOut, "placement-soft-opt-out", "", "Optional true or false for placement_soft_opt_out")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "Plan without writing")
	pg.AddCommand(cmd)
}
