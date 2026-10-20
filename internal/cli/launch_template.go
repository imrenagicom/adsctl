package cli

import (
	"adsctl/internal/output"
	"github.com/spf13/cobra"
	"os"
)

const trafficTemplate = `version: 1
kind: meta-launch
name: website-traffic
assets:
  - key: banner
    type: image
    file: ./banner.jpg
campaigns:
  - key: traffic
    advantage_budget: true
    payload:
      name: Website traffic
      objective: OUTCOME_TRAFFIC
      daily_budget: 2500
      bid_strategy: LOWEST_COST_WITHOUT_CAP
      special_ad_categories: []
      status: PAUSED
adsets:
  - key: audience
    campaign_ref: traffic
    advantage_placements: true
    payload:
      name: Indonesia traffic
      billing_event: IMPRESSIONS
      optimization_goal: LINK_CLICKS
      targeting:
        geo_locations:
          countries: [ID]
      status: PAUSED
creatives:
  - key: banner
    image_ref: banner
    payload:
      name: Banner
      object_story_spec:
        page_id: 'REPLACE_WITH_PAGE_ID'
        link_data:
          link: https://example.com/product
          message: Explore our product
          name: Product headline
          call_to_action:
            type: LEARN_MORE
            value:
              link: https://example.com/product
ads:
  - key: banner
    adset_ref: audience
    creative_ref: banner
    payload:
      name: Banner ad
      status: PAUSED
`

func (a *App) addLaunchTemplate(g *cobra.Command) {
	var objective, format, save string
	cmd := action("template", "Write an editable website traffic/image launch template", "local", func(c *cobra.Command, _ []string) error {
		if objective != "traffic" || format != "image" {
			return output.Fail("USAGE", "The starter template supports traffic and image; native manifests support other payloads.", 2)
		}
		if save == "" {
			return a.emit(c, map[string]any{"format": "yaml", "template": trafficTemplate})
		}
		file, err := os.OpenFile(save, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return output.Fail("OUTPUT_EXISTS", "Template output exists or cannot be created.", 2)
		}
		defer func() { _ = file.Close() }()
		if _, err = file.WriteString(trafficTemplate); err != nil {
			return err
		}
		if err = file.Sync(); err != nil {
			return err
		}
		return a.emit(c, map[string]any{"file": save})
	})
	cmd.Flags().StringVar(&objective, "objective", "traffic", "Starter objective")
	cmd.Flags().StringVar(&format, "format", "image", "Starter media format")
	cmd.Flags().StringVar(&save, "save", "", "New template path")
	g.AddCommand(cmd)
}
