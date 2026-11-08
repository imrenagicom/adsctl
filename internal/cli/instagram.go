package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"adsctl/internal/platform/meta/graph"
	"github.com/spf13/cobra"
)

// Instagram publishing uses the Instagram Graph API through Facebook Login
// (graph.facebook.com and rupload.facebook.com) with the profile's token. The
// token needs instagram_basic and instagram_content_publish for the IG user,
// plus instagram_manage_contents to delete media and instagram_manage_insights
// to read insights (see instagram_insights.go).

const (
	igMaxCaption     = 2200
	igMaxHashtags    = 30
	igMaxMentions    = 20
	igMaxChildren    = 20
	igMediaFields    = "id,caption,media_type,media_product_type,permalink,timestamp,like_count,comments_count"
	igContainerField = "id,status_code,status"
)

var (
	igHashtagRE = regexp.MustCompile(`(^|\s)#[^\s#]+`)
	igMentionRE = regexp.MustCompile(`(^|\s)@[A-Za-z0-9._]+`)
	igUserRE    = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)
)

type igSpec struct {
	Type, ImageURL, VideoURL, File, Caption, CaptionFile string
	AltText, CoverURL, AudioName, LocationID             string
	ThumbOffset                                          int
	ShareToFeed                                          bool
	Collaborators, Children, Items                       []string
}

// igRuntime reuses Meta profile resolution; Instagram commands are scoped by
// IG user instead of ad account. Writes require explicit --profile, a
// write-mode profile and an explicit --ig-user.
func (a *App) igRuntime(c *cobra.Command, user *string, userRequired, write bool) (*runtime, error) {
	if user != nil && *user == "" && !write {
		*user = os.Getenv("ADSCTL_IG_USER")
	}
	if userRequired {
		if write && user != nil && *user == "" && os.Getenv("ADSCTL_IG_USER") != "" {
			return nil, output.Fail("WRITE_SCOPE", "Instagram writes require an explicit --ig-user; ADSCTL_IG_USER applies to reads only.", 4)
		}
		if user == nil || *user == "" {
			return nil, output.Fail("USAGE", "Select an Instagram user with --ig-user (see adsctl ig accounts list).", 2)
		}
		if err := config.ID(*user); err != nil {
			return nil, err
		}
	}
	r, err := a.runtime(c, false, false)
	if err != nil {
		return nil, err
	}
	if write && (!c.Flags().Changed("profile") || r.Mode != "write" || (userRequired && !c.Flags().Changed("ig-user"))) {
		r.Cancel()
		return nil, output.Fail("WRITE_SCOPE", "Instagram writes require explicit --profile, --ig-user and a write-mode profile.", 4)
	}
	a.meta["platform"] = "instagram"
	delete(a.meta, "account_id")
	if user != nil && *user != "" {
		a.meta["ig_user_id"] = *user
	}
	return r, nil
}

func (a *App) addInstagram(root *cobra.Command) {
	ig := group("ig", "Publish and inspect Instagram professional account content")
	root.AddCommand(ig)

	accounts := group("accounts", "Discover Instagram professional accounts")
	ig.AddCommand(accounts)
	accounts.AddCommand(action("list", "List Pages and their linked Instagram professional accounts", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.igRuntime(c, nil, false, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		return a.list(c, r, "me/accounts", url.Values{"fields": {"id,name,instagram_business_account{id,username,name}"}}, graph.PageOptions{Scope: r.Profile, All: true, PageSize: 100, MaxItems: 1000})
	}))

	var authUser string
	auth := group("auth", "Check Instagram publishing and insights access")
	ig.AddCommand(auth)
	check := action("check", "Verify the IG user is readable and report publishing and insights permissions and quota", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.igRuntime(c, &authUser, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		user, err := r.Client.Request(r.Ctx, "GET", authUser, url.Values{"fields": {"id,username,name,media_count"}})
		if err != nil {
			return err
		}
		result := map[string]any{"ig_user": user.Value, "mode": r.Mode, "content_publish_permission": "unknown", "content_delete_permission": "unknown", "insights_permission": "unknown"}
		if perms, e := r.Client.Request(r.Ctx, "GET", "me/permissions", nil); e == nil {
			granted := []string{}
			rows, _ := perms.Value["data"].([]any)
			for _, row := range rows {
				m, _ := row.(map[string]any)
				if m["status"] == "granted" {
					granted = append(granted, fmt.Sprint(m["permission"]))
				}
			}
			result["granted_permissions"] = granted
			result["content_publish_permission"] = slices.Contains(granted, "instagram_content_publish")
			result["content_delete_permission"] = slices.Contains(granted, "instagram_manage_contents")
			result["insights_permission"] = slices.Contains(granted, "instagram_manage_insights")
			missing := []string{}
			for _, p := range []string{"instagram_basic", "instagram_content_publish", "instagram_manage_contents", "instagram_manage_insights", "pages_show_list", "pages_read_engagement"} {
				if !slices.Contains(granted, p) {
					missing = append(missing, p)
				}
			}
			result["missing_permissions"] = missing
		}
		if limit, e := r.Client.Request(r.Ctx, "GET", authUser+"/content_publishing_limit", url.Values{"fields": {"config,quota_usage"}}); e == nil {
			result["publishing_limit"] = limit.Value
		}
		return a.emit(c, result)
	})
	check.Flags().StringVar(&authUser, "ig-user", "", "Instagram user ID (or ADSCTL_IG_USER)")
	auth.AddCommand(check)

	var limitUser string
	limit := action("limit", "Read the 24-hour content publishing quota", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.igRuntime(c, &limitUser, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		v, err := r.Client.Request(r.Ctx, "GET", limitUser+"/content_publishing_limit", url.Values{"fields": {"config,quota_usage"}})
		if err != nil {
			return err
		}
		return a.emit(c, v.Value)
	})
	limit.Flags().StringVar(&limitUser, "ig-user", "", "Instagram user ID (or ADSCTL_IG_USER)")
	ig.AddCommand(limit)

	a.addInstagramMedia(ig)
	a.addInstagramContainers(ig)
	a.addInstagramPublish(ig)
	a.addInstagramPost(ig)
	a.addInstagramInsights(ig)
}

func (a *App) addInstagramMedia(ig *cobra.Command) {
	media := group("media", "Inspect published Instagram media")
	ig.AddCommand(media)
	var user, fields, cursor string
	var all bool
	var size, maxItems int
	list := action("list", "List published media for an IG user with bounded pagination", "read", func(c *cobra.Command, _ []string) error {
		r, err := a.igRuntime(c, &user, true, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		return a.list(c, r, user+"/media", url.Values{"fields": {fields}}, graph.PageOptions{Scope: r.Profile + "|" + user, Cursor: cursor, All: all, PageSize: size, MaxItems: maxItems})
	})
	f := list.Flags()
	f.StringVar(&user, "ig-user", "", "Instagram user ID (or ADSCTL_IG_USER)")
	f.StringVar(&fields, "fields", igMediaFields, "Native media fields")
	f.BoolVar(&all, "all", false, "Fetch all pages within max-items")
	f.StringVar(&cursor, "cursor", "", "Scoped continuation cursor")
	f.IntVar(&size, "page-size", 25, "Requested page size, 1..1000")
	f.IntVar(&maxItems, "max-items", 1000, "Maximum returned rows")
	media.AddCommand(list)
	var getFields string
	get := action("get ID", "Read published Instagram media by ID", "read", func(c *cobra.Command, args []string) error {
		r, err := a.igRuntime(c, nil, false, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = config.ID(args[0]); err != nil {
			return err
		}
		v, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {getFields}})
		if err != nil {
			return err
		}
		return a.emit(c, v.Value)
	})
	get.Args = cobra.ExactArgs(1)
	get.Flags().StringVar(&getFields, "fields", igMediaFields, "Native media fields")
	media.AddCommand(get)

	var delUser string
	var delDry bool
	del := action("delete ID", "Permanently delete a published post, reel, story or whole carousel", "write", func(c *cobra.Command, args []string) error {
		if err := config.ID(args[0]); err != nil {
			return err
		}
		r, err := a.igRuntime(c, &delUser, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		receipt := map[string]any{"media_id": args[0], "ig_user_id": delUser, "dry_run": delDry, "state": "planned"}
		target, err := igOwned(r, delUser, args[0])
		if target != nil {
			receipt["media"] = target
		}
		if err != nil {
			receipt["state"] = "failed"
			return a.partial(c, receipt, err)
		}
		if delDry {
			return a.emit(c, receipt)
		}
		v, err := r.Client.Request(r.Ctx, "DELETE", args[0], nil)
		if err != nil {
			if output.AsError(err).Exit != 10 {
				receipt["state"] = "failed"
				return a.partial(c, receipt, err)
			}
			// Unknown acknowledgment: reconcile by reading, never by repeating.
			if _, e := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {"id"}}); e != nil && output.AsError(e).Code == "VALIDATION" {
				receipt["state"] = "deleted"
				return a.emit(c, receipt)
			}
			receipt["state"] = "unknown"
			e := output.AsError(err)
			e.SuggestedAction = "Run adsctl ig media get " + args[0] + "; a not-found error means the delete succeeded."
			return a.partial(c, receipt, e)
		}
		if v.Value["success"] != true {
			receipt["state"] = "unknown"
			return a.partial(c, receipt, output.Fail("MUTATION_UNKNOWN", "Delete returned no success acknowledgment; read the media before retrying.", 10))
		}
		receipt["state"], receipt["deleted_id"] = "deleted", v.Value["deleted_id"]
		return a.emit(c, receipt)
	})
	del.Args = cobra.ExactArgs(1)
	del.Flags().StringVar(&delUser, "ig-user", "", "Instagram user ID that owns the media")
	del.Flags().BoolVar(&delDry, "dry-run", false, "Read and verify the media without deleting it")
	media.AddCommand(del)
}

// igOwned reads the media and confirms the selected IG user owns it, so a
// mistyped ID cannot delete another account's content.
func igOwned(r *runtime, user, id string) (map[string]any, error) {
	m, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {"id,owner,username,permalink,media_product_type,timestamp"}})
	if err != nil {
		return nil, err
	}
	if owner, ok := m.Value["owner"].(map[string]any); ok && fmt.Sprint(owner["id"]) == user {
		return m.Value, nil
	}
	if name := fmt.Sprint(m.Value["username"]); m.Value["username"] != nil {
		u, e := r.Client.Request(r.Ctx, "GET", user, url.Values{"fields": {"id,username"}})
		if e != nil {
			return m.Value, e
		}
		if fmt.Sprint(u.Value["username"]) == name {
			return m.Value, nil
		}
	}
	return m.Value, output.Fail("ACCOUNT_MISMATCH", "Media does not belong to the selected Instagram user.", 4)
}

func addSpecFlags(c *cobra.Command, s *igSpec, types string) {
	f := c.Flags()
	f.StringVar(&s.Type, "type", "", "Media type: "+types)
	f.StringVar(&s.ImageURL, "image-url", "", "Public JPEG URL Meta fetches (image, story, carousel-item)")
	f.StringVar(&s.VideoURL, "video-url", "", "Public video URL Meta fetches (reel, story, carousel-item)")
	f.StringVar(&s.File, "file", "", "Local video file sent by resumable upload (reel, story)")
	f.StringVar(&s.Caption, "caption", "", "Caption text (max 2200 characters, 30 hashtags, 20 mentions)")
	f.StringVar(&s.CaptionFile, "caption-file", "", "Read the caption from a UTF-8 file (- for stdin)")
	f.StringVar(&s.AltText, "alt-text", "", "Accessibility alt text (image posts only)")
	f.StringVar(&s.CoverURL, "cover-url", "", "Public JPEG cover URL (reel)")
	f.IntVar(&s.ThumbOffset, "thumb-offset", 0, "Cover frame offset in milliseconds (reel)")
	f.BoolVar(&s.ShareToFeed, "share-to-feed", true, "Also show the reel in the profile feed grid (reel)")
	f.StringVar(&s.AudioName, "audio-name", "", "Name for the reel's original audio (reel)")
	f.StringVar(&s.LocationID, "location-id", "", "Facebook Page ID of a location to tag")
	f.StringSliceVar(&s.Collaborators, "collaborator", nil, "Instagram username to invite as collaborator (repeatable, max 3)")
}

// igParams validates a spec locally and returns native container parameters
// plus the local file path when the container needs a resumable upload.
func (a *App) igParams(c *cobra.Command, s *igSpec) (url.Values, string, error) {
	p := url.Values{}
	fail := func(m string) (url.Values, string, error) { return nil, "", output.Fail("USAGE", m, 2) }
	set := func(names ...string) []string {
		var out []string
		for _, n := range names {
			if c.Flags().Changed(n) {
				out = append(out, "--"+n)
			}
		}
		return out
	}
	allowed := map[string][]string{
		"image":         {"image-url", "caption", "caption-file", "alt-text", "location-id", "collaborator"},
		"reel":          {"video-url", "file", "caption", "caption-file", "cover-url", "thumb-offset", "share-to-feed", "audio-name", "location-id", "collaborator"},
		"story":         {"image-url", "video-url", "file"},
		"carousel-item": {"image-url", "video-url"},
		"carousel":      {"child", "item", "caption", "caption-file", "location-id", "collaborator"},
	}
	ok, known := allowed[s.Type]
	if !known {
		return fail("Type must be image, reel, story, carousel-item or carousel.")
	}
	all := []string{"image-url", "video-url", "file", "caption", "caption-file", "alt-text", "cover-url", "thumb-offset", "share-to-feed", "audio-name", "location-id", "collaborator", "child", "item"}
	for _, n := range all {
		if c.Flags().Lookup(n) != nil && c.Flags().Changed(n) && !slices.Contains(ok, n) {
			return fail("--" + n + " is not supported for type " + s.Type + ".")
		}
	}
	sources := set("image-url", "video-url", "file")
	if s.Type != "carousel" && len(sources) != 1 {
		var options []string
		for _, n := range []string{"image-url", "video-url", "file"} {
			if slices.Contains(ok, n) {
				options = append(options, "--"+n)
			}
		}
		return fail("Type " + s.Type + " requires exactly one media source: " + strings.Join(options, ", ") + ".")
	}
	for name, v := range map[string]string{"image-url": s.ImageURL, "video-url": s.VideoURL, "cover-url": s.CoverURL} {
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fail("--" + name + " must be an absolute public http(s) URL that Meta can fetch.")
		}
	}
	if s.File != "" {
		info, err := os.Stat(s.File)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > graph.MaxUploadSize {
			return nil, "", output.Fail("INPUT", "Specify a nonempty regular video --file at most 1 GiB.", 2)
		}
		if ext := strings.ToLower(filepath.Ext(s.File)); ext != ".mp4" && ext != ".mov" {
			return fail("--file must be an .mp4 or .mov video; images need a public --image-url.")
		}
	}
	caption, err := a.igCaption(s)
	if err != nil {
		return nil, "", err
	}
	if caption != "" {
		p.Set("caption", caption)
	}
	if s.AltText != "" {
		if utf8.RuneCountInString(s.AltText) > 1000 {
			return fail("--alt-text must be at most 1000 characters.")
		}
		p.Set("alt_text", s.AltText)
	}
	if s.LocationID != "" {
		if err = config.ID(s.LocationID); err != nil {
			return nil, "", err
		}
		p.Set("location_id", s.LocationID)
	}
	if len(s.Collaborators) > 0 {
		if len(s.Collaborators) > 3 {
			return fail("At most 3 collaborators are supported.")
		}
		for i, u := range s.Collaborators {
			u = strings.TrimPrefix(strings.TrimSpace(u), "@")
			if !igUserRE.MatchString(u) {
				return fail("Collaborators must be Instagram usernames.")
			}
			s.Collaborators[i] = u
		}
		b, _ := json.Marshal(s.Collaborators)
		p.Set("collaborators", string(b))
	}
	upload := ""
	switch s.Type {
	case "image":
		p.Set("image_url", s.ImageURL)
	case "reel":
		p.Set("media_type", "REELS")
		if s.File != "" {
			p.Set("upload_type", "resumable")
			upload = s.File
		} else {
			p.Set("video_url", s.VideoURL)
		}
		if s.CoverURL != "" {
			p.Set("cover_url", s.CoverURL)
		}
		if c.Flags().Changed("thumb-offset") {
			if s.ThumbOffset < 0 {
				return fail("--thumb-offset must not be negative.")
			}
			p.Set("thumb_offset", strconv.Itoa(s.ThumbOffset))
		}
		if c.Flags().Changed("share-to-feed") {
			p.Set("share_to_feed", strconv.FormatBool(s.ShareToFeed))
		}
		if s.AudioName != "" {
			p.Set("audio_name", s.AudioName)
		}
	case "story":
		p.Set("media_type", "STORIES")
		switch {
		case s.ImageURL != "":
			p.Set("image_url", s.ImageURL)
		case s.VideoURL != "":
			p.Set("video_url", s.VideoURL)
		default:
			p.Set("upload_type", "resumable")
			upload = s.File
		}
	case "carousel-item":
		p.Set("is_carousel_item", "true")
		if s.ImageURL != "" {
			p.Set("image_url", s.ImageURL)
		} else {
			p.Set("media_type", "VIDEO")
			p.Set("video_url", s.VideoURL)
		}
	case "carousel":
		p.Set("media_type", "CAROUSEL")
		if c.Flags().Lookup("child") != nil {
			for _, id := range s.Children {
				if err = config.ID(id); err != nil {
					return nil, "", err
				}
			}
			if len(s.Children) < 2 || len(s.Children) > igMaxChildren {
				return fail("A carousel needs 2 to 20 child containers.")
			}
			p.Set("children", strings.Join(s.Children, ","))
		}
	}
	if s.ImageURL != "" {
		if ext := strings.ToLower(filepath.Ext(strings.SplitN(s.ImageURL, "?", 2)[0])); ext != "" && ext != ".jpg" && ext != ".jpeg" {
			return fail("Instagram publishing accepts JPEG images only; convert the image and host it as .jpg.")
		}
	}
	return p, upload, nil
}

func (a *App) igCaption(s *igSpec) (string, error) {
	caption := s.Caption
	if s.CaptionFile != "" {
		if s.Caption != "" {
			return "", output.Fail("USAGE", "Choose --caption or --caption-file, not both.", 2)
		}
		var b []byte
		var err error
		if s.CaptionFile == "-" {
			b, err = io.ReadAll(io.LimitReader(a.In, 64*1024))
		} else {
			b, err = os.ReadFile(s.CaptionFile)
		}
		if err != nil {
			return "", output.Fail("INPUT", "Cannot read caption file.", 2)
		}
		if !utf8.Valid(b) {
			return "", output.Fail("INPUT", "Caption file must be UTF-8.", 2)
		}
		caption = strings.TrimSpace(string(b))
	}
	if n := utf8.RuneCountInString(caption); n > igMaxCaption {
		return "", output.Fail("USAGE", fmt.Sprintf("Caption has %d characters; Instagram allows %d.", n, igMaxCaption), 2)
	}
	if n := len(igHashtagRE.FindAllString(caption, -1)); n > igMaxHashtags {
		return "", output.Fail("USAGE", fmt.Sprintf("Caption has %d hashtags; Instagram allows %d.", n, igMaxHashtags), 2)
	}
	if n := len(igMentionRE.FindAllString(caption, -1)); n > igMaxMentions {
		return "", output.Fail("USAGE", fmt.Sprintf("Caption has %d mentions; Instagram allows %d.", n, igMaxMentions), 2)
	}
	return caption, nil
}

func (a *App) addInstagramContainers(ig *cobra.Command) {
	containers := group("containers", "Create and inspect unpublished Instagram media containers")
	ig.AddCommand(containers)

	var user string
	var dry bool
	var retry igUploadRetry
	s := &igSpec{}
	create := action("create", "Create an unpublished media container (nothing is public until publish)", "write", func(c *cobra.Command, _ []string) error {
		params, upload, err := a.igParams(c, s)
		if err != nil {
			return err
		}
		r, err := a.igRuntime(c, &user, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		receipt := map[string]any{"type": s.Type, "ig_user_id": user, "params": params, "dry_run": dry, "state": "planned", "public": false}
		if upload != "" {
			receipt["upload_file"] = upload
		}
		if dry {
			return a.emit(c, receipt)
		}
		if _, err = igCreateUpload(r, user, params, upload, retry, receipt); err != nil {
			return a.partial(c, receipt, err)
		}
		return a.emit(c, receipt)
	})
	create.Flags().StringVar(&user, "ig-user", "", "Instagram user ID")
	create.Flags().BoolVar(&dry, "dry-run", false, "Validate and print the container request without calling Meta")
	create.Flags().StringSliceVar(&s.Children, "child", nil, "Carousel child container ID (repeatable, 2 to 20)")
	addUploadRetryFlags(create, &retry)
	addSpecFlags(create, s, "image, reel, story, carousel-item, carousel")
	containers.AddCommand(create)

	var file string
	var upDry bool
	upload := action("upload ID", "Send a local video to a resumable container", "write", func(c *cobra.Command, args []string) error {
		if err := config.ID(args[0]); err != nil {
			return err
		}
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > graph.MaxUploadSize {
			return output.Fail("INPUT", "Specify a nonempty regular video --file at most 1 GiB.", 2)
		}
		r, err := a.igRuntime(c, nil, false, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		receipt := map[string]any{"container_id": args[0], "file": file, "size": info.Size(), "dry_run": upDry, "state": "planned", "public": false}
		if upDry {
			return a.emit(c, receipt)
		}
		v, err := r.Client.UploadInstagramVideo(r.Ctx, args[0], file)
		if v.Value != nil {
			receipt["response"] = v.Value
		}
		if err != nil {
			receipt["state"] = igFailState(err)
			return a.partial(c, receipt, err)
		}
		receipt["state"] = "uploaded"
		return a.emit(c, receipt)
	})
	upload.Args = cobra.ExactArgs(1)
	upload.Flags().StringVar(&file, "file", "", "Local .mp4 or .mov video")
	upload.Flags().BoolVar(&upDry, "dry-run", false, "Inspect the file without uploading")
	containers.AddCommand(upload)

	get := action("get ID", "Read a container's processing status", "read", func(c *cobra.Command, args []string) error {
		r, err := a.igRuntime(c, nil, false, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = config.ID(args[0]); err != nil {
			return err
		}
		v, err := r.Client.Request(r.Ctx, "GET", args[0], url.Values{"fields": {igContainerField}})
		if err != nil {
			return err
		}
		return a.emit(c, v.Value)
	})
	get.Args = cobra.ExactArgs(1)
	containers.AddCommand(get)

	var interval time.Duration
	wait := action("wait ID", "Wait until a container is FINISHED (ready to publish)", "read", func(c *cobra.Command, args []string) error {
		igLongTimeout(c, a)
		r, err := a.igRuntime(c, nil, false, false)
		if err != nil {
			return err
		}
		defer r.Cancel()
		if err = config.ID(args[0]); err != nil {
			return err
		}
		v, err := igWait(r, args[0], interval)
		if err != nil {
			return a.partial(c, v, err)
		}
		return a.emit(c, v)
	})
	wait.Args = cobra.ExactArgs(1)
	wait.Flags().DurationVar(&interval, "poll-interval", 10*time.Second, "Status polling interval")
	containers.AddCommand(wait)
}

func (a *App) addInstagramPublish(ig *cobra.Command) {
	var user string
	var dry bool
	publish := action("publish CONTAINER_ID", "Publish a FINISHED container to the IG user's public profile", "write", func(c *cobra.Command, args []string) error {
		if err := config.ID(args[0]); err != nil {
			return err
		}
		r, err := a.igRuntime(c, &user, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		receipt := map[string]any{"container_id": args[0], "ig_user_id": user, "dry_run": dry, "state": "planned", "public": true}
		if dry {
			return a.emit(c, receipt)
		}
		if err = igPublish(r, user, args[0], receipt); err != nil {
			return a.partial(c, receipt, err)
		}
		return a.emit(c, receipt)
	})
	publish.Args = cobra.ExactArgs(1)
	publish.Flags().StringVar(&user, "ig-user", "", "Instagram user ID")
	publish.Flags().BoolVar(&dry, "dry-run", false, "Print the publish plan without calling Meta")
	ig.AddCommand(publish)
}

func (a *App) addInstagramPost(ig *cobra.Command) {
	var user string
	var dry, noPublish bool
	var interval time.Duration
	var retry igUploadRetry
	s := &igSpec{}
	post := action("post", "Create, upload, wait for and publish an Instagram post, reel, story or carousel", "write", func(c *cobra.Command, _ []string) error {
		if s.Type == "carousel-item" {
			return output.Fail("USAGE", "Type must be image, reel, story or carousel.", 2)
		}
		params, upload, err := a.igParams(c, s)
		if err != nil {
			return err
		}
		var items []url.Values
		if s.Type == "carousel" {
			if len(s.Items) < 2 || len(s.Items) > igMaxChildren {
				return output.Fail("USAGE", "A carousel post needs 2 to 20 --item values.", 2)
			}
			for _, item := range s.Items {
				p, e := igItem(item)
				if e != nil {
					return e
				}
				items = append(items, p)
			}
		}
		igLongTimeout(c, a)
		r, err := a.igRuntime(c, &user, true, true)
		if err != nil {
			return err
		}
		defer r.Cancel()
		receipt := map[string]any{"type": s.Type, "ig_user_id": user, "params": params, "dry_run": dry, "publish": !noPublish, "state": "planned"}
		if upload != "" {
			receipt["upload_file"] = upload
		}
		if len(items) > 0 {
			receipt["items"] = items
		}
		if dry {
			return a.emit(c, receipt)
		}
		if len(items) > 0 {
			children := []string{}
			for _, p := range items {
				id, e := igCreate(r, user, p)
				if e != nil {
					receipt["children"], receipt["state"] = children, igFailState(e)
					return a.partial(c, receipt, e)
				}
				children = append(children, id)
				receipt["children"] = children
				if p.Get("media_type") == "VIDEO" {
					if v, e := igWait(r, id, interval); e != nil {
						receipt["child_status"], receipt["state"] = v, "child_failed"
						return a.partial(c, receipt, e)
					}
				}
			}
			params.Set("children", strings.Join(children, ","))
		}
		id, err := igCreateUpload(r, user, params, upload, retry, receipt)
		if err != nil {
			return a.partial(c, receipt, err)
		}
		status, err := igWait(r, id, interval)
		receipt["container_status"] = status
		if err != nil {
			receipt["state"] = "processing_failed"
			if output.AsError(err).Exit == 7 {
				receipt["state"] = "processing"
			}
			return a.partial(c, receipt, err)
		}
		receipt["state"] = "ready"
		if noPublish {
			return a.emit(c, receipt)
		}
		if err = igPublish(r, user, id, receipt); err != nil {
			return a.partial(c, receipt, err)
		}
		return a.emit(c, receipt)
	})
	post.Flags().StringVar(&user, "ig-user", "", "Instagram user ID")
	post.Flags().BoolVar(&dry, "dry-run", false, "Validate and print the plan without calling Meta")
	post.Flags().BoolVar(&noPublish, "no-publish", false, "Stop after the container is FINISHED; publish later with adsctl ig publish")
	post.Flags().DurationVar(&interval, "poll-interval", 10*time.Second, "Container status polling interval")
	post.Flags().StringArrayVar(&s.Items, "item", nil, "Carousel item as image=URL or video=URL (repeatable, 2 to 20, in order)")
	addUploadRetryFlags(post, &retry)
	addSpecFlags(post, s, "image, reel, story, carousel")
	ig.AddCommand(post)
}

func igItem(item string) (url.Values, error) {
	kind, raw, ok := strings.Cut(item, "=")
	u, err := url.Parse(raw)
	if !ok || (kind != "image" && kind != "video") || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, output.Fail("USAGE", "--item must be image=URL or video=URL with a public http(s) URL.", 2)
	}
	p := url.Values{"is_carousel_item": {"true"}}
	if kind == "image" {
		if ext := strings.ToLower(filepath.Ext(u.Path)); ext != "" && ext != ".jpg" && ext != ".jpeg" {
			return nil, output.Fail("USAGE", "Instagram publishing accepts JPEG images only; convert the image and host it as .jpg.", 2)
		}
		p.Set("image_url", raw)
	} else {
		p.Set("media_type", "VIDEO")
		p.Set("video_url", raw)
	}
	return p, nil
}

// igLongTimeout raises the default operation deadline for commands that wait
// for video processing; an explicit --timeout always wins.
func igLongTimeout(c *cobra.Command, a *App) {
	if !c.Flags().Changed("timeout") && a.opts.Timeout < 10*time.Minute {
		a.opts.Timeout = 10 * time.Minute
	}
}

// igUploadRetry bounds how often a failed local-file upload is retried.
type igUploadRetry struct {
	Attempts int
	Wait     time.Duration
}

func addUploadRetryFlags(c *cobra.Command, r *igUploadRetry) {
	c.Flags().IntVar(&r.Attempts, "upload-attempts", 3, "Attempts for a local --file upload; each retry uses a new private container (1 disables retries)")
	c.Flags().DurationVar(&r.Wait, "upload-retry-wait", time.Minute, "Wait before the first upload retry; doubles for each further retry")
}

// igUploadRetrySafe reports whether a failed upload may be retried. A
// container stays private until media_publish, so any upload failure that is
// not a credential, permission, input or deadline problem is safe to repeat
// against a fresh container: nothing can be published twice.
func igUploadRetrySafe(err error) bool {
	switch output.AsError(err).Exit {
	case 6, 8, 10:
		return true
	}
	return false
}

// igCreateUpload creates the container and, for a local file, uploads it.
// Meta's upload host sometimes rejects a valid file (HTTP 400 with
// debug_info ProcessingFailedError, marked not retriable) and accepts the
// same bytes minutes later, so a rejected upload is retried on a new
// container with backoff. Every attempt is recorded in the receipt.
func igCreateUpload(r *runtime, user string, params url.Values, upload string, retry igUploadRetry, receipt map[string]any) (string, error) {
	attempts := retry.Attempts
	if attempts < 1 || upload == "" {
		attempts = 1
	}
	wait := retry.Wait
	var tries []map[string]any
	for k := 1; ; k++ {
		id, err := igCreate(r, user, params)
		if err != nil {
			receipt["state"] = igFailState(err)
			return "", err
		}
		receipt["container_id"], receipt["state"] = id, "created"
		if upload == "" {
			return id, nil
		}
		_, err = r.Client.UploadInstagramVideo(r.Ctx, id, upload)
		if err == nil {
			receipt["state"] = "uploaded"
			if len(tries) > 0 {
				receipt["upload_attempts"] = append(tries, map[string]any{"container_id": id, "result": "uploaded"})
			}
			return id, nil
		}
		e := output.AsError(err)
		tries = append(tries, map[string]any{"container_id": id, "result": "failed", "code": e.Code, "message": e.Message, "http_status": e.HTTPStatus})
		receipt["upload_attempts"], receipt["state"] = tries, "created_upload_failed"
		if k >= attempts || !igUploadRetrySafe(err) {
			if k > 1 {
				e.Message = fmt.Sprintf("%s (attempt %d of %d; every container is unpublished)", strings.TrimSuffix(e.Message, "."), k, attempts)
			}
			return "", e
		}
		if r.Client.Log != nil {
			_, _ = fmt.Fprintf(r.Client.Log, "upload attempt %d of %d failed (%s); retrying on a new container in %s\n", k, attempts, e.Message, wait)
		}
		select {
		case <-r.Ctx.Done():
			return "", output.Fail("TIMEOUT", "Deadline reached while waiting to retry the upload; nothing was published.", 7)
		case <-time.After(wait):
		}
		wait *= 2
	}
}

func igFailState(err error) string {
	if output.AsError(err).Exit == 10 {
		return "unknown"
	}
	return "failed"
}

func igCreate(r *runtime, user string, params url.Values) (string, error) {
	v, err := r.Client.Request(r.Ctx, "POST", user+"/media", params)
	if err != nil {
		e := output.AsError(err)
		if e.Exit == 10 {
			e.SuggestedAction = "Containers are unpublished and expire after 24 hours; an unknown create is safe to repeat. Nothing was published."
		}
		return "", e
	}
	id := fmt.Sprint(v.Value["id"])
	if config.ID(id) != nil {
		return "", output.Fail("MUTATION_UNKNOWN", "Container create returned no ID; nothing was published.", 10)
	}
	return id, nil
}

func igStatus(r *runtime, id string) (map[string]any, error) {
	v, err := r.Client.Request(r.Ctx, "GET", id, url.Values{"fields": {igContainerField}})
	return v.Value, err
}

func igWait(r *runtime, id string, interval time.Duration) (map[string]any, error) {
	if interval <= 0 {
		return nil, output.Fail("USAGE", "Poll interval must be positive.", 2)
	}
	for {
		v, err := igStatus(r, id)
		if err != nil {
			return v, err
		}
		switch v["status_code"] {
		case "FINISHED", "PUBLISHED":
			return v, nil
		case "ERROR":
			return v, output.Fail("CONTAINER_FAILED", "Instagram could not process the container: "+fmt.Sprint(v["status"]), 8)
		case "EXPIRED":
			return v, output.Fail("CONTAINER_EXPIRED", "Container expired (not published within 24 hours); create a new one.", 8)
		}
		timer := time.NewTimer(interval)
		select {
		case <-r.Ctx.Done():
			timer.Stop()
			e := output.Fail("TIMEOUT", "Container is still processing; run adsctl ig containers wait "+id+" and publish when FINISHED.", 7)
			return v, e
		case <-timer.C:
		}
	}
}

// igPublish refuses containers that are not FINISHED, so a repeated command
// cannot post the same container twice, and reconciles unknown acknowledgments
// by reading the container status instead of retrying.
func igPublish(r *runtime, user, id string, receipt map[string]any) error {
	status, err := igStatus(r, id)
	if err != nil {
		receipt["state"] = "failed"
		return err
	}
	receipt["container_status"] = status
	switch status["status_code"] {
	case "FINISHED":
	case "PUBLISHED":
		receipt["state"] = "already_published"
		e := output.Fail("ALREADY_PUBLISHED", "Container was already published; it was not published again.", 5)
		e.SuggestedAction = "Find it with adsctl ig media list --ig-user " + user + "."
		return e
	default:
		receipt["state"] = "not_ready"
		return output.Fail("CONTAINER_NOT_READY", fmt.Sprintf("Container status is %v; publish requires FINISHED.", status["status_code"]), 5)
	}
	if limit, e := r.Client.Request(r.Ctx, "GET", user+"/content_publishing_limit", url.Values{"fields": {"config,quota_usage"}}); e == nil {
		receipt["publishing_limit"] = limit.Value
		if quotaExhausted(limit.Value) {
			receipt["state"] = "quota_exhausted"
			return output.Fail("QUOTA_EXHAUSTED", "The IG user has used its 24-hour publishing quota.", 6)
		}
	}
	receipt["state"] = "publishing"
	v, err := r.Client.Request(r.Ctx, "POST", user+"/media_publish", url.Values{"creation_id": {id}})
	if err != nil {
		if output.AsError(err).Exit != 10 {
			receipt["state"] = "failed"
			return err
		}
		// Unknown acknowledgment: never retry, only reconcile.
		if after, e := igStatus(r, id); e == nil {
			receipt["container_status"] = after
			if after["status_code"] == "PUBLISHED" {
				receipt["state"] = "published"
				receipt["media_id"] = nil
				receipt["note"] = "Publish acknowledgment was lost, but the container reports PUBLISHED. Find the media ID with adsctl ig media list."
				return nil
			}
		}
		receipt["state"] = "unknown"
		e := output.AsError(err)
		e.SuggestedAction = "Do not publish again yet. Run adsctl ig containers get " + id + "; PUBLISHED means the post is live."
		return e
	}
	media := fmt.Sprint(v.Value["id"])
	receipt["media_id"], receipt["state"] = media, "published"
	if config.ID(media) == nil {
		if m, e := r.Client.Request(r.Ctx, "GET", media, url.Values{"fields": {"id,permalink,media_type,media_product_type,timestamp"}}); e == nil {
			receipt["media"] = m.Value
		}
	}
	return nil
}

func quotaExhausted(v map[string]any) bool {
	rows, _ := v["data"].([]any)
	if len(rows) == 0 {
		return false
	}
	row, _ := rows[0].(map[string]any)
	cfg, _ := row["config"].(map[string]any)
	used, e1 := strconv.Atoi(fmt.Sprint(row["quota_usage"]))
	total, e2 := strconv.Atoi(fmt.Sprint(cfg["quota_total"]))
	return e1 == nil && e2 == nil && total > 0 && used >= total
}
