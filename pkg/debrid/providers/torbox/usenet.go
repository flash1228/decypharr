package torbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	json "github.com/bytedance/sonic"
	debridCommon "github.com/sirrobot01/decypharr/pkg/debrid/common"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
)

const torboxUsenetScheme = "torbox-usenet://"

// SupportsUsenet returns true when the TorBox account is on the Pro plan.
// The profile is lazy-fetched and cached after the first call to GetProfile.
func (tb *Torbox) SupportsUsenet() bool {
	profile, err := tb.GetProfile()
	if err != nil {
		return false
	}
	return profile.Type == "pro"
}

// SubmitNZB uploads NZB content to TorBox's usenet service and returns the usenet download ID.
func (tb *Torbox) SubmitNZB(ctx context.Context, nzbContent []byte, name string) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", name)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := fw.Write(nzbContent); err != nil {
		return "", fmt.Errorf("failed to write NZB content: %w", err)
	}
	_ = w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tb.Host+"/api/usenet/createusenetdownload", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := tb.nzbUploadClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("torbox usenet submit: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read usenet submit response: %w", err)
	}
	tb.logger.Debug().Str("body", string(body)).Msg("TorBox usenet create response")

	var result addNZBResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode usenet submit response: %w", err)
	}

	if !result.Success || result.Data == nil {
		return "", fmt.Errorf("torbox usenet submit failed: %s", result.Detail)
	}

	return strconv.Itoa(result.Data.Id), nil
}

// maxActiveUsenetDownloads is the TorBox Pro concurrent usenet download queue limit.
const maxActiveUsenetDownloads = 6

// GetActiveUsenetCount returns how many usenet downloads are currently active (queued or downloading).
func (tb *Torbox) GetActiveUsenetCount(ctx context.Context) (int, error) {
	var res usenetListResponse
	resp, err := tb.doGet("/api/usenet/mylist", map[string]string{"bypass_cache": "true"}, &res)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("torbox usenet mylist: HTTP %d", resp.StatusCode)
	}
	count := 0
	if res.Data != nil {
		for _, dl := range *res.Data {
			if dl.Active {
				count++
			}
		}
	}
	return count, nil
}

// GetUsenetDownload fetches the current state of a TorBox usenet download by ID.
// /mylist?id=N returns a single object, not an array.
func (tb *Torbox) GetUsenetDownload(ctx context.Context, id string) (*usenetInfo, error) {
	var res usenetInfoResponse
	resp, err := tb.doGet("/api/usenet/mylist", map[string]string{"id": id}, &res)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("torbox usenet mylist: HTTP %d", resp.StatusCode)
	}
	if !res.Success || res.Data == nil {
		return nil, fmt.Errorf("torbox usenet download %s not found: %s", id, res.Detail)
	}
	return res.Data, nil
}

// WaitForUsenetCached polls until the usenet download is cached/finished or timeout elapses.
// onProgress is called on each poll with the current progress (0.0–1.0); may be nil.
// Returns the download info (with file list) on success, an error on failure or timeout.
func (tb *Torbox) WaitForUsenetCached(ctx context.Context, id string, timeout time.Duration, onProgress func(float64)) (*debridCommon.UsenetDownload, error) {
	deadline := time.Now().Add(timeout)
	const pollInterval = 5 * time.Second

	for {
		info, err := tb.GetUsenetDownload(ctx, id)
		if err != nil {
			return nil, err
		}

		if onProgress != nil {
			onProgress(info.Progress)
		}

		// TorBox sets download_finished=true before it finishes extracting/processing
		// files. Wait until files are populated and state leaves "processing".
		ready := (info.DownloadFinished || info.Cached) &&
			len(info.Files) > 0 &&
			info.DownloadState != "processing"
		if ready {
			tb.logger.Info().
				Str("usenet_id", id).
				Str("name", info.Name).
				Float64("progress", info.Progress*100).
				Int("file_count", len(info.Files)).
				Msg("TorBox usenet download ready")
			for _, f := range info.Files {
				tb.logger.Debug().Str("file", f.Name).Int64("size", f.Size).Msg("TorBox usenet file")
			}
			return toUsenetDownload(info), nil
		}

		switch info.DownloadState {
		case "error", "failed", "virus", "timeout":
			return nil, fmt.Errorf("torbox usenet download %s failed with state %q", id, info.DownloadState)
		case "paused":
			// TorBox auto-manages its queue; system-paused downloads resume automatically
			// when a download slot is free. The controlusenetdownload API cannot unblock
			// system-managed pauses (returns 400). Just wait.
			tb.logger.Debug().Str("usenet_id", id).Msg("TorBox usenet download queued (paused) — waiting for slot")
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("torbox usenet download %s did not complete within %s (state: %s, progress: %.1f%%)",
				id, timeout, info.DownloadState, info.Progress*100)
		}

		tb.logger.Debug().
			Str("usenet_id", id).
			Str("state", info.DownloadState).
			Float64("progress_pct", info.Progress*100).
			Msg("Waiting for TorBox usenet download")

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// controlUsenetDownload sends a control operation to TorBox for a usenet download.
// Valid operations: "Delete", "Pause", "Resume".
// Note: TorBox auto-manages its download queue; system-paused downloads cannot
// be resumed via this endpoint (returns 400). Only user-paused downloads respond.
func (tb *Torbox) controlUsenetDownload(ctx context.Context, id string, operation string) error {
	payload := map[string]interface{}{"usenet_id": id, "operation": operation}
	resp, err := tb.doPost("/api/usenet/controlusenetdownload", payload)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("torbox controlusenetdownload %s: HTTP %d", operation, resp.StatusCode)
	}
	return nil
}

// DeleteUsenetDownload removes a usenet download from TorBox.
func (tb *Torbox) DeleteUsenetDownload(ctx context.Context, id string) error {
	return tb.controlUsenetDownload(ctx, id, "Delete")
}

// fetchUsenetDownloadLink fetches a CDN download URL for a specific file in a usenet download.
// Called by fetchDownloadLink when file.Link carries the torbox-usenet:// scheme.
func (tb *Torbox) fetchUsenetDownloadLink(acc *account.Account, usenetID, fileID string) (string, error) {
	var res DownloadLinksResponse
	resp, err := tb.doGet("/api/usenet/requestdl", map[string]string{
		"token":     acc.Token,
		"usenet_id": usenetID,
		"file_id":   fileID,
	}, &res)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("torbox usenet requestdl: HTTP %d", resp.StatusCode)
	}
	if !res.Success || res.Data == nil || *res.Data == "" {
		return "", fmt.Errorf("torbox usenet requestdl: empty CDN URL: %s", res.Detail)
	}
	return *res.Data, nil
}

// BuildUsenetLink constructs the internal torbox-usenet:// link stored in ProviderFile.Link.
func BuildUsenetLink(usenetID, fileID string) string {
	return torboxUsenetScheme + usenetID + "/" + fileID
}

// ParseUsenetLink extracts usenet download ID and file ID from a torbox-usenet:// link.
func ParseUsenetLink(link string) (usenetID, fileID string, ok bool) {
	if !strings.HasPrefix(link, torboxUsenetScheme) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(link, torboxUsenetScheme), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// toUsenetDownload converts the TorBox API response into the shared UsenetDownload type.
func toUsenetDownload(info *usenetInfo) *debridCommon.UsenetDownload {
	dl := &debridCommon.UsenetDownload{
		ID:    strconv.Itoa(info.Id),
		Name:  info.Name,
		Size:  info.Size,
		Files: make([]debridCommon.UsenetFile, 0, len(info.Files)),
	}
	for _, f := range info.Files {
		dl.Files = append(dl.Files, debridCommon.UsenetFile{
			ID:   strconv.Itoa(f.Id),
			Name: f.Name,
			Size: f.Size,
			Path: f.Name,
		})
	}
	return dl
}
