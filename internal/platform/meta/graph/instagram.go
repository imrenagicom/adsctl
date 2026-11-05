package graph

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"adsctl/internal/config"
	"adsctl/internal/output"
)

// RuploadURL is Meta's host for Instagram resumable video uploads.
const RuploadURL = "https://rupload.facebook.com"

// UploadInstagramVideo sends a local video to an unpublished Instagram media
// container created with upload_type=resumable. The container stays private
// until media_publish, so a failed or unknown upload never publishes anything;
// it can be retried against the same container or replaced with a new one.
func (c *Client) UploadInstagramVideo(ctx context.Context, containerID, path string) (Response, error) {
	if err := config.ID(containerID); err != nil {
		return Response{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Response{}, output.Fail("INPUT", "Cannot open upload file.", 2)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > MaxUploadSize {
		return Response{}, output.Fail("INPUT", "Upload requires a nonempty regular file at most 1 GiB.", 2)
	}
	if err = c.pace(ctx); err != nil {
		return Response{}, err
	}
	endpoint := c.BaseURL
	if endpoint == "https://graph.facebook.com" {
		endpoint = RuploadURL
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Host == "" {
		return Response{}, output.Fail("CONFIG", "Invalid upload base URL.", 2)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/ig-api-upload/" + c.Version + "/" + containerID
	if c.AppSecret != "" {
		h := hmac.New(sha256.New, []byte(c.AppSecret))
		_, _ = h.Write([]byte(c.Token))
		base.RawQuery = url.Values{"appsecret_proof": {hex.EncodeToString(h.Sum(nil))}}.Encode()
	}
	// The transfer is bounded by the operation deadline, not the per-request
	// timeout, because a large Reel cannot be sent within a single GET budget.
	req, err := http.NewRequestWithContext(ctx, "POST", base.String(), f)
	if err != nil {
		return Response{}, output.Fail("INTERNAL", "Cannot construct upload request.", 1)
	}
	req.ContentLength = info.Size()
	req.Header.Set("Authorization", "OAuth "+c.Token)
	req.Header.Set("offset", "0")
	req.Header.Set("file_size", strconv.FormatInt(info.Size(), 10))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "adsctl/0.1")
	if c.Log != nil {
		_, _ = fmt.Fprintf(c.Log, "POST ig-api-upload/%s/%s (%d bytes; credentials redacted)\n", c.Version, containerID, info.Size())
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		e := output.Fail("UPLOAD_UNKNOWN", "Instagram video upload was interrupted; the container is unpublished.", 10)
		e.SuggestedAction = "Check the container with adsctl ig containers get " + containerID + "; retry the upload or create a new container. Nothing was published."
		return Response{}, e
	}
	defer func() { _ = resp.Body.Close() }()
	r, err := c.readResponse("POST", resp)
	if err != nil {
		e := output.AsError(err)
		if e.Code == "MUTATION_UNKNOWN" {
			e.Code = "UPLOAD_UNKNOWN"
		}
		e.SuggestedAction = "The container is unpublished. Check it with adsctl ig containers get " + containerID + " before retrying the upload."
		return r, e
	}
	if r.Value["success"] != true {
		message := "Instagram upload did not acknowledge success."
		e := output.Fail("UPLOAD_FAILED", message, 8)
		if debug, ok := r.Value["debug_info"].(map[string]any); ok {
			if m := fmt.Sprint(debug["message"]); debug["message"] != nil && m != "" {
				e.Message = "Instagram upload failed: " + m
			}
			e.Provider = debug
			e.Retryable = debug["retriable"] == true
		}
		e.SuggestedAction = "The container is unpublished. Retry with adsctl ig containers upload " + containerID + " --file PATH or create a new container."
		return r, e
	}
	return r, nil
}
