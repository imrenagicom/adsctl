package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"adsctl/internal/output"
)

func igEnvelope(t *testing.T, out *bytes.Buffer) (output.Envelope, map[string]any) {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err, out.String())
	}
	data, _ := env.Data.(map[string]any)
	return env, data
}

func TestInstagramDryRunMakesNoRequest(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("dry run made a request: %s", r.URL.Path) })
	video := filepath.Join(t.TempDir(), "reel.mp4")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	caption := filepath.Join(t.TempDir(), "caption.md")
	if err := os.WriteFile(caption, []byte("Belajar Mandarin bareng Gochi #hsk\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "post", "--profile", "test", "--ig-user", "1784", "--type", "reel", "--file", video, "--caption-file", caption, "--share-to-feed=false", "--collaborator", "@gochi.app", "--dry-run"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	env, data := igEnvelope(t, out)
	params, _ := data["params"].(map[string]any)
	if !env.OK || data["state"] != "planned" || env.Meta["platform"] != "instagram" || env.Meta["ig_user_id"] != "1784" {
		t.Fatal(out.String())
	}
	for key, want := range map[string]string{"media_type": "REELS", "upload_type": "resumable", "caption": "Belajar Mandarin bareng Gochi #hsk", "share_to_feed": "false", "collaborators": `["gochi.app"]`} {
		got, _ := params[key].([]any)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s = %v", key, params[key])
		}
	}
}

func TestInstagramValidation(t *testing.T) {
	a, _, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("invalid input made a request: %s", r.URL.Path) })
	base := []string{"--config", cfg, "ig", "post", "--profile", "test", "--ig-user", "1784", "--dry-run"}
	cases := map[string][]string{
		"png image":         {"--type", "image", "--image-url", "https://cdn.example.com/a.png"},
		"no source":         {"--type", "reel"},
		"two sources":       {"--type", "reel", "--video-url", "https://cdn.example.com/a.mp4", "--image-url", "https://cdn.example.com/a.jpg"},
		"story caption":     {"--type", "story", "--image-url", "https://cdn.example.com/a.jpg", "--caption", "hi"},
		"local image":       {"--type", "image", "--file", "a.jpg"},
		"relative url":      {"--type", "image", "--image-url", "/a.jpg"},
		"long caption":      {"--type", "image", "--image-url", "https://cdn.example.com/a.jpg", "--caption", strings.Repeat("a", 2201)},
		"too many hashtags": {"--type", "image", "--image-url", "https://cdn.example.com/a.jpg", "--caption", strings.Repeat("#tag ", 31)},
		"one carousel item": {"--type", "carousel", "--item", "image=https://cdn.example.com/a.jpg"},
		"bad item":          {"--type", "carousel", "--item", "gif=https://cdn.example.com/a.gif", "--item", "image=https://cdn.example.com/b.jpg"},
		"unknown type":      {"--type", "carousel-item", "--image-url", "https://cdn.example.com/a.jpg"},
	}
	for name, args := range cases {
		if err := executeTest(a, append(append([]string{}, base...), args...)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestInstagramWriteScope(t *testing.T) {
	a, _, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unscoped write made a request: %s", r.URL.Path)
	})
	t.Setenv("ADSCTL_IG_USER", "1784")
	t.Setenv("ADSCTL_PROFILE", "test") // env selection is not explicit enough for writes
	for _, args := range [][]string{
		{"ig", "publish", "555", "--ig-user", "1784"},
		{"ig", "publish", "555", "--profile", "test"},
		{"ig", "post", "--type", "image", "--image-url", "https://cdn.example.com/a.jpg", "--profile", "test"},
	} {
		err := executeTest(a, append([]string{"--config", cfg}, args...))
		if err == nil || output.AsError(err).Code != "WRITE_SCOPE" {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestInstagramReelUploadWaitPublish(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	polls := 0
	published := false
	var uploaded []byte
	a, out, errOut, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "POST" && r.URL.Path == "/v25.0/1784/media":
			_ = r.ParseForm()
			if r.Form.Get("media_type") != "REELS" || r.Form.Get("upload_type") != "resumable" || r.Form.Get("caption") != "Halo" {
				t.Errorf("container params: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"555"}`)
		case r.Method == "POST" && r.URL.Path == "/ig-api-upload/v25.0/555":
			if r.Header.Get("Authorization") != "OAuth fixture-token" || r.Header.Get("offset") != "0" || r.Header.Get("file_size") != "5" {
				t.Errorf("upload headers: %v", r.Header)
			}
			uploaded, _ = io.ReadAll(r.Body)
			_, _ = fmt.Fprint(w, `{"success":true,"message":"Upload successful."}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/555":
			polls++
			status := "IN_PROGRESS"
			if published {
				status = "PUBLISHED"
			} else if polls > 1 {
				status = "FINISHED"
			}
			_, _ = fmt.Fprintf(w, `{"id":"555","status_code":"%s"}`, status)
		case r.Method == "GET" && r.URL.Path == "/v25.0/1784/content_publishing_limit":
			_, _ = fmt.Fprint(w, `{"data":[{"config":{"quota_total":100,"quota_duration":86400},"quota_usage":3}]}`)
		case r.Method == "POST" && r.URL.Path == "/v25.0/1784/media_publish":
			_ = r.ParseForm()
			if r.Form.Get("creation_id") != "555" {
				t.Errorf("publish params: %v", r.Form)
			}
			published = true
			_, _ = fmt.Fprint(w, `{"id":"9001"}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/9001":
			_, _ = fmt.Fprint(w, `{"id":"9001","permalink":"https://www.instagram.com/reel/abc/","media_product_type":"REELS"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	video := filepath.Join(t.TempDir(), "reel.mp4")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "post", "--profile", "test", "--ig-user", "1784", "--type", "reel", "--file", video, "--caption", "Halo", "--poll-interval", "1ms"})
	if err != nil {
		t.Fatal(err, out.String(), errOut.String())
	}
	_, data := igEnvelope(t, out)
	if data["state"] != "published" || data["media_id"] != "9001" || data["container_id"] != "555" || string(uploaded) != "video" {
		t.Fatal(out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("fixture-token")) {
		t.Fatal("token leaked")
	}

	// Publishing the same container again must be refused, not re-posted.
	out.Reset()
	err = executeTest(a, []string{"--config", cfg, "ig", "publish", "555", "--profile", "test", "--ig-user", "1784"})
	if err == nil || output.AsError(err).Code != "ALREADY_PUBLISHED" {
		t.Fatal(err, out.String())
	}
	n := 0
	for _, c := range calls {
		if c == "POST /v25.0/1784/media_publish" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("media_publish called %d times", n)
	}
}

func TestInstagramCarouselNoPublish(t *testing.T) {
	children := 0
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v25.0/1784/media":
			_ = r.ParseForm()
			if r.Form.Get("is_carousel_item") == "true" {
				children++
				_, _ = fmt.Fprintf(w, `{"id":"10%d"}`, children)
				return
			}
			if r.Form.Get("media_type") != "CAROUSEL" || r.Form.Get("children") != "101,102" {
				t.Errorf("carousel params: %v", r.Form)
			}
			_, _ = fmt.Fprint(w, `{"id":"200"}`)
		case r.Method == "GET":
			_, _ = fmt.Fprintf(w, `{"id":"%s","status_code":"FINISHED"}`, strings.TrimPrefix(r.URL.Path, "/v25.0/"))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "post", "--profile", "test", "--ig-user", "1784", "--type", "carousel", "--item", "image=https://cdn.example.com/1.jpg", "--item", "video=https://cdn.example.com/2.mp4", "--caption", "Swipe", "--no-publish", "--poll-interval", "1ms"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	_, data := igEnvelope(t, out)
	if data["state"] != "ready" || data["container_id"] != "200" || children != 2 {
		t.Fatal(out.String())
	}
}

func TestInstagramPublishUnknownReconciles(t *testing.T) {
	published := false
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/555":
			status := "FINISHED"
			if published {
				status = "PUBLISHED"
			}
			_, _ = fmt.Fprintf(w, `{"id":"555","status_code":"%s"}`, status)
		case "/v25.0/1784/content_publishing_limit":
			_, _ = fmt.Fprint(w, `{"data":[]}`)
		case "/v25.0/1784/media_publish":
			published = true
			w.WriteHeader(502)
		}
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "publish", "555", "--profile", "test", "--ig-user", "1784"})
	if err != nil {
		t.Fatal(err, out.String())
	}
	_, data := igEnvelope(t, out)
	if data["state"] != "published" {
		t.Fatal(out.String())
	}
}

func TestInstagramQuotaExhausted(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/555":
			_, _ = fmt.Fprint(w, `{"id":"555","status_code":"FINISHED"}`)
		case "/v25.0/1784/content_publishing_limit":
			_, _ = fmt.Fprint(w, `{"data":[{"config":{"quota_total":100},"quota_usage":100}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	err := executeTest(a, []string{"--config", cfg, "ig", "publish", "555", "--profile", "test", "--ig-user", "1784"})
	if err == nil || output.AsError(err).Code != "QUOTA_EXHAUSTED" {
		t.Fatal(err, out.String())
	}
}

func TestInstagramUploadFailureKeepsContainerPrivate(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/1784/media":
			_, _ = fmt.Fprint(w, `{"id":"555"}`)
		case "/ig-api-upload/v25.0/555":
			_, _ = fmt.Fprint(w, `{"debug_info":{"retriable":true,"type":"ProcessingFailedError","message":"Request processing failed"}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	video := filepath.Join(t.TempDir(), "story.mov")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "containers", "create", "--profile", "test", "--ig-user", "1784", "--type", "story", "--file", video, "--upload-attempts", "1"})
	if err == nil || output.AsError(err).Code != "UPLOAD_FAILED" || !output.AsError(err).Retryable {
		t.Fatal(err, out.String())
	}
	_, data := igEnvelope(t, out)
	if data["state"] != "created_upload_failed" || data["container_id"] != "555" {
		t.Fatal(out.String())
	}
}

func TestInstagramMediaDeleteChecksOwner(t *testing.T) {
	deletes := 0
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v25.0/9001":
			_, _ = fmt.Fprint(w, `{"id":"9001","owner":{"id":"1784"},"username":"gochi_mandarin"}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/9002":
			_, _ = fmt.Fprint(w, `{"id":"9002","username":"someone_else"}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/1784":
			_, _ = fmt.Fprint(w, `{"id":"1784","username":"gochi_mandarin"}`)
		case r.Method == "DELETE" && r.URL.Path == "/v25.0/9001":
			deletes++
			_, _ = fmt.Fprint(w, `{"success":true,"deleted_id":"9001"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	base := []string{"--config", cfg, "ig", "media", "delete", "--profile", "test", "--ig-user", "1784"}
	if err := executeTest(a, append(append([]string{}, base...), "9001", "--dry-run")); err != nil || deletes != 0 {
		t.Fatal(err, deletes, out.String())
	}
	out.Reset()
	err := executeTest(a, append(append([]string{}, base...), "9002"))
	if err == nil || output.AsError(err).Code != "ACCOUNT_MISMATCH" || deletes != 0 {
		t.Fatal(err, deletes)
	}
	out.Reset()
	if err = executeTest(a, append(append([]string{}, base...), "9001")); err != nil {
		t.Fatal(err, out.String())
	}
	_, data := igEnvelope(t, out)
	if data["state"] != "deleted" || data["deleted_id"] != "9001" || deletes != 1 {
		t.Fatal(out.String())
	}
}

// Meta's upload host answers a rejected upload with HTTP 400 and a debug_info
// object (no Graph "error"); the reason must reach the user.
func TestInstagramUploadHTTP400ShowsReason(t *testing.T) {
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/1784/media":
			_, _ = fmt.Fprint(w, `{"id":"555"}`)
		case "/ig-api-upload/v25.0/555":
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"debug_info":{"retriable":false,"type":"ProcessingFailedError","message":"Request processing failed"}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	video := filepath.Join(t.TempDir(), "reel.mp4")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "containers", "create", "--profile", "test", "--ig-user", "1784", "--type", "reel", "--file", video, "--upload-attempts", "1"})
	e := output.AsError(err)
	if err == nil || e.Code != "UPLOAD_FAILED" || e.HTTPStatus != 400 || !strings.Contains(e.Message, "ProcessingFailedError: Request processing failed") {
		t.Fatal(err, out.String())
	}
	if _, data := igEnvelope(t, out); data["state"] != "created_upload_failed" {
		t.Fatal(out.String())
	}
}

// A rejected upload is retried on a new private container and only the
// container that uploaded is published.
func TestInstagramUploadRetriesOnNewContainer(t *testing.T) {
	var mu sync.Mutex
	created, publishedID := 0, ""
	a, out, errOut, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "POST" && r.URL.Path == "/v25.0/1784/media":
			created++
			_, _ = fmt.Fprintf(w, `{"id":"55%d"}`, created)
		case r.Method == "POST" && r.URL.Path == "/ig-api-upload/v25.0/551":
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"debug_info":{"retriable":false,"type":"ProcessingFailedError","message":"Request processing failed"}}`)
		case r.Method == "POST" && r.URL.Path == "/ig-api-upload/v25.0/552":
			_, _ = fmt.Fprint(w, `{"success":true,"message":"Upload Successful."}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/552":
			status := "FINISHED"
			if publishedID != "" {
				status = "PUBLISHED"
			}
			_, _ = fmt.Fprintf(w, `{"id":"552","status_code":"%s"}`, status)
		case r.Method == "GET" && r.URL.Path == "/v25.0/1784/content_publishing_limit":
			_, _ = fmt.Fprint(w, `{"data":[{"config":{"quota_total":100,"quota_duration":86400},"quota_usage":3}]}`)
		case r.Method == "POST" && r.URL.Path == "/v25.0/1784/media_publish":
			_ = r.ParseForm()
			publishedID = r.Form.Get("creation_id")
			_, _ = fmt.Fprint(w, `{"id":"9001"}`)
		case r.Method == "GET" && r.URL.Path == "/v25.0/9001":
			_, _ = fmt.Fprint(w, `{"id":"9001","permalink":"https://www.instagram.com/reel/abc/","media_product_type":"REELS"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	video := filepath.Join(t.TempDir(), "reel.mp4")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "post", "--profile", "test", "--ig-user", "1784", "--type", "reel", "--file", video, "--poll-interval", "1ms", "--upload-retry-wait", "1ms"})
	if err != nil {
		t.Fatal(err, out.String(), errOut.String())
	}
	_, data := igEnvelope(t, out)
	tries, _ := data["upload_attempts"].([]any)
	if data["state"] != "published" || data["container_id"] != "552" || publishedID != "552" || created != 2 || len(tries) != 2 {
		t.Fatal(out.String())
	}
	if first, _ := tries[0].(map[string]any); first["container_id"] != "551" || first["code"] != "UPLOAD_FAILED" {
		t.Fatal(out.String())
	}
}

// Credential and permission failures are not retried.
func TestInstagramUploadAuthFailureNotRetried(t *testing.T) {
	created := 0
	a, out, _, cfg := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v25.0/1784/media":
			created++
			_, _ = fmt.Fprint(w, `{"id":"555"}`)
		case "/ig-api-upload/v25.0/555":
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Invalid OAuth access token.","code":190}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	video := filepath.Join(t.TempDir(), "reel.mp4")
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	err := executeTest(a, []string{"--config", cfg, "ig", "containers", "create", "--profile", "test", "--ig-user", "1784", "--type", "reel", "--file", video, "--upload-retry-wait", "1ms"})
	if err == nil || output.AsError(err).Code != "AUTH_INVALID" || created != 1 {
		t.Fatal(err, created, out.String())
	}
}
