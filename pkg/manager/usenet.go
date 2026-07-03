package manager

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/sirrobot01/decypharr/internal/config"
	debridCommon "github.com/sirrobot01/decypharr/pkg/debrid/common"
	debridTypes "github.com/sirrobot01/decypharr/pkg/debrid/types"
	"github.com/sirrobot01/decypharr/pkg/debrid/providers/torbox"
	"github.com/sirrobot01/decypharr/pkg/storage"
	"github.com/sirrobot01/decypharr/pkg/usenet/parser"
)

// AddNewNZB processes an NZB file and stores it as a storage.Entry.
// When a TorBox Pro client is configured (and usenet_backend != "nntp"), the NZB is
// submitted to TorBox's usenet API instead of streaming over NNTP. Falls back to
// NNTP transparently on any TorBox error.
func (m *Manager) AddNewNZB(ctx context.Context, req *ImportRequest) (string, error) {
	// Attempt TorBox usenet API path first (Pro plan auto-detected).
	if id, err := m.tryTorboxUsenet(ctx, req); id != "" || err != nil {
		return id, err
	}

	if m.usenet == nil {
		return "", fmt.Errorf("usenet not configured")
	}

	m.logger.Info().
		Str("name", req.Name).
		Str("category", req.Arr.Name).
		Msg("Adding new NZB to usenet")

	// Parse NZB through usenet client
	meta, groups, err := m.usenet.Parse(ctx, req.Name, req.NZBContent, req.Arr.Name)
	if err != nil {
		return "", fmt.Errorf("usenet process failed: %w", err)
	}

	// Create storage.Entry
	entry := &storage.Entry{
		InfoHash:         meta.ID,
		Name:             meta.Name,
		OriginalFilename: meta.Name,
		Size:             meta.TotalSize,
		Protocol:         config.ProtocolNZB,
		Bytes:            meta.TotalSize,
		Category:         req.Arr.Name,
		SavePath:         filepath.Join(req.DownloadFolder, req.Arr.Name),
		Status:           debridTypes.TorrentStatusDownloading,
		State:            storage.EntryStateDownloading,
		Progress:         0,
		Action:           req.Action,
		CallbackURL:      req.CallBackUrl,
		SkipMultiSeason:  req.SkipMultiSeason,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		AddedOn:          time.Now(),
		Providers:        make(map[string]*storage.ProviderEntry),
		Files:            make(map[string]*storage.File),
		Tags:             []string{},
	}

	entry.ContentPath = entry.DownloadPath()
	_ = entry.AddUsenetProvider(meta)
	entry.ActiveProvider = "usenet"
	entry.UpdatedAt = time.Now()
	entry.State = storage.EntryStateDownloading
	entry.Status = debridTypes.TorrentStatusDownloading
	if err := m.queue.Add(entry); err != nil {
		return "", fmt.Errorf("failed to add nzb to queue: %w", err)
	}

	// Submit job to unbounded worker pool queue (never blocks)
	m.nzbQueue.Push(&nzbJob{entry: entry, meta: meta, groups: groups})
	m.logger.Debug().Str("name", entry.Name).Int("queued", m.nzbQueue.Len()).Msg("NZB added to processing queue")

	return meta.ID, nil
}

func (m *Manager) processNZB(ctx context.Context, entry *storage.Entry, metadata *storage.NZB) error {
	// Add files using logical streamable files
	for _, file := range metadata.Files {
		tFile := &storage.File{
			Name:     file.Name,
			Size:     file.Size,
			InfoHash: entry.InfoHash,
			AddedOn:  entry.AddedOn,
		}
		entry.Files[file.Name] = tFile
	}
	// Mark as complete
	if placement := entry.GetActiveProvider(); placement != nil {
		now := time.Now()
		placement.DownloadedAt = &now
		placement.Progress = 1.0
	}
	entry.Size = metadata.TotalSize
	entry.Progress = 1.0
	entry.UpdatedAt = time.Now()
	_ = m.queue.Update(entry)

	for _, file := range metadata.Files {
		go func(f storage.NZBFile) {
			cacheCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			_ = m.usenet.PreCache(cacheCtx, metadata.ID, f.Name) // This will fetch head and tail of the file
		}(file)
	}

	if len(entry.Files) == 0 {
		return fmt.Errorf("nzb has no files")
	}

	go m.processAction(entry)
	return nil
}

// processNewNzb processes a new NZB entry after it has been added to the usenet client
func (m *Manager) processNewNzb(entry *storage.Entry, metadata *storage.NZB, groups map[string]*parser.FileGroup) error {
	// Create context with timeout for processing
	ctx, cancel := context.WithTimeout(context.Background(), m.usenetTimeout)
	defer cancel()

	updatedNZB, err := m.usenet.Process(ctx, metadata, groups)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return fmt.Errorf("usenet processing timed out after %s: %w", m.usenetTimeout, err)
		}
		return fmt.Errorf("failed to process nzb: %w", err)
	}

	metadata = updatedNZB
	return m.processNZB(ctx, entry, metadata)
}

// HasUsenet returns true if usenet is configured
func (m *Manager) HasUsenet() bool {
	return m.usenet != nil
}

// UsenetStats returns usenet client statistics
func (m *Manager) UsenetStats() map[string]interface{} {
	if m.usenet == nil {
		return nil
	}
	return m.usenet.Stats()
}

// SpeedTestRequest represents a speed test request payload
type SpeedTestRequest struct {
	Protocol string `json:"protocol"` // "nntp" or "debrid"
	Provider string `json:"provider"` // provider host/identifier
}

// SpeedTestResponse represents a speed test result
type SpeedTestResponse struct {
	Provider  string  `json:"provider"`
	Protocol  string  `json:"protocol"`
	SpeedMBps float64 `json:"speed_mbps"`
	LatencyMs int64   `json:"latency_ms"`
	BytesRead int64   `json:"bytes_read"`
	TestedAt  string  `json:"tested_at"`
	Error     string  `json:"error,omitempty"`
}

// SpeedTest runs a speed test for a specific provider based on protocol
func (m *Manager) SpeedTest(ctx context.Context, req SpeedTestRequest) SpeedTestResponse {
	switch req.Protocol {
	case "nntp":
		if m.usenet == nil {
			return SpeedTestResponse{
				Provider: req.Provider,
				Protocol: req.Protocol,
				Error:    "usenet not configured",
			}
		}
		result := m.usenet.SpeedTest(ctx, req.Provider)
		return SpeedTestResponse{
			Provider:  result.Provider,
			Protocol:  req.Protocol,
			SpeedMBps: result.SpeedMBps,
			LatencyMs: result.LatencyMs,
			BytesRead: result.BytesRead,
			TestedAt:  result.TestedAt.Format("2006-01-02T15:04:05Z07:00"),
			Error:     result.Error,
		}
	case "debrid":
		// Look up debrid client by provider name
		client, exists := m.clients.Load(req.Provider)
		if !exists {
			return SpeedTestResponse{
				Provider: req.Provider,
				Protocol: req.Protocol,
				Error:    "debrid provider not found: " + req.Provider,
			}
		}
		result := client.SpeedTest(ctx)

		// Store the result for persistence (so it shows up in stats)
		if result.Error == "" {
			m.debridSpeedTestResults.Store(req.Provider, result)
		}

		return SpeedTestResponse{
			Provider:  result.Provider,
			Protocol:  req.Protocol,
			SpeedMBps: result.SpeedMBps,
			LatencyMs: result.LatencyMs,
			BytesRead: result.BytesRead,
			TestedAt:  result.TestedAt.Format("2006-01-02T15:04:05Z07:00"),
			Error:     result.Error,
		}
	default:
		return SpeedTestResponse{
			Provider: req.Provider,
			Protocol: req.Protocol,
			Error:    "unknown protocol: " + req.Protocol,
		}
	}
}

// tryTorboxUsenet attempts to route the NZB through TorBox's usenet API.
// Returns ("", nil) to signal "fall through to NNTP" when conditions are not met or on
// recoverable failure. Returns a non-empty id on success, or a non-nil error on hard failure.
func (m *Manager) tryTorboxUsenet(ctx context.Context, req *ImportRequest) (string, error) {
	var (
		nc         debridCommon.NZBClient
		debridName string
	)

	m.clients.Range(func(name string, c debridCommon.Client) bool {
		dc := c.Config()
		// Respect explicit "nntp" override — skip this provider.
		if dc.UsenetBackend == "nntp" {
			return true
		}
		// Only TorBox implements NZBClient.
		if _, ok := c.(*torbox.Torbox); !ok {
			return true
		}
		candidate, ok := c.(debridCommon.NZBClient)
		if !ok {
			return true
		}
		if dc.UsenetBackend == "torbox" || candidate.SupportsUsenet() {
			nc = candidate
			debridName = name
			return false // stop iteration
		}
		return true
	})

	if nc == nil {
		return "", nil // no eligible TorBox Pro client found
	}

	return m.addNZBViaTorbox(ctx, req, nc, debridName)
}

// addNZBViaTorbox queues the entry immediately and returns the ID to the caller
// (so Sonarr/Radarr get a fast ACK), then submits to TorBox and polls for cache
// completion entirely in the background.
func (m *Manager) addNZBViaTorbox(ctx context.Context, req *ImportRequest, nc debridCommon.NZBClient, debridName string) (string, error) {
	// Create and register the entry before doing any network I/O so the
	// SABnzbd HTTP response returns immediately — TorBox submit can take 30-120s.
	entryID := uuid.New().String()
	now := time.Now()
	entry := &storage.Entry{
		InfoHash:         entryID,
		Name:             req.Name,
		OriginalFilename: req.Name,
		Protocol:         config.ProtocolNZB,
		Category:         req.Arr.Name,
		SavePath:         filepath.Join(req.DownloadFolder, req.Arr.Name),
		Status:           debridTypes.TorrentStatusDownloading,
		State:            storage.EntryStateDownloading,
		Progress:         0,
		Action:           req.Action,
		CallbackURL:      req.CallBackUrl,
		SkipMultiSeason:  req.SkipMultiSeason,
		ActiveProvider:   debridName,
		CreatedAt:        now,
		UpdatedAt:        now,
		AddedOn:          now,
		Providers:        make(map[string]*storage.ProviderEntry),
		Files:            make(map[string]*storage.File),
		Tags:             []string{"torbox-usenet"},
	}
	entry.ContentPath = entry.DownloadPath()

	if err := m.queue.Add(entry); err != nil {
		return "", fmt.Errorf("failed to add TorBox usenet entry to queue: %w", err)
	}

	m.logger.Info().Str("name", req.Name).Str("debrid", debridName).Msg("NZB queued for TorBox usenet, submitting in background")

	// Hold processingEntries for the full goroutine lifetime so processQueuedEntries
	// doesn't race with us while SubmitNZB is in-flight (before the ID is stored).
	m.processingEntries.Store(entry.InfoHash, struct{}{})

	nzbContent := req.NZBContent // capture before req may be reused
	nzbName := req.Name

	go func() {
		defer m.processingEntries.Delete(entry.InfoHash)
		bgCtx, cancel := context.WithTimeout(context.Background(), m.usenetTimeout)
		defer cancel()

		// Check active queue depth before submitting — TorBox allows max 6 concurrent usenet downloads.
		if activeCount, countErr := nc.GetActiveUsenetCount(bgCtx); countErr == nil {
			m.logger.Info().Int("active", activeCount).Str("name", nzbName).Msg("TorBox usenet active download count")
			if activeCount >= 6 {
				m.logger.Warn().Int("active", activeCount).Str("name", nzbName).
					Msg("TorBox usenet queue full (6 active) — marking entry errored; clear queue before retrying")
				entry.State = storage.EntryStateError
				entry.Status = debridTypes.TorrentStatusError
				entry.UpdatedAt = time.Now()
				_ = m.queue.Update(entry)
				return
			}
		}

		usenetID, err := nc.SubmitNZB(bgCtx, nzbContent, nzbName)
		if err != nil {
			m.logger.Warn().Err(err).Str("name", nzbName).Msg("TorBox usenet submit failed — marking entry errored")
			entry.State = storage.EntryStateError
			entry.Status = debridTypes.TorrentStatusError
			entry.UpdatedAt = time.Now()
			_ = m.queue.Update(entry)
			return
		}

		// Persist the usenet ID immediately so polling can resume after a restart.
		entry.Providers[debridName] = &storage.ProviderEntry{
			Provider: debridName,
			ID:       usenetID,
			AddedAt:  now,
			Status:   debridTypes.TorrentStatusDownloading,
			Progress: 0,
			Files:    make(map[string]*storage.ProviderFile),
		}
		entry.UpdatedAt = time.Now()
		_ = m.queue.Update(entry)

		m.logger.Info().Str("name", nzbName).Str("usenet_id", usenetID).Msg("NZB submitted to TorBox usenet, waiting for cache")

		dl, err := nc.WaitForUsenetCached(bgCtx, usenetID, m.usenetTimeout, func(progress float64) {
			entry.Progress = progress
			entry.UpdatedAt = time.Now()
			_ = m.queue.Update(entry)
		})
		if err != nil {
			m.logger.Warn().Err(err).Str("name", nzbName).Str("usenet_id", usenetID).
				Msg("TorBox usenet cache wait failed — marking entry errored")
			_ = nc.DeleteUsenetDownload(bgCtx, usenetID)
			entry.State = storage.EntryStateError
			entry.Status = debridTypes.TorrentStatusError
			entry.UpdatedAt = time.Now()
			_ = m.queue.Update(entry)
			return
		}

		m.finalizeTorboxUsenetEntry(entry, dl, usenetID, debridName)
	}()

	return entryID, nil
}

// resumeTorboxUsenetPolling re-attaches a WaitForUsenetCached goroutine to an entry
// that was left in-progress when decypharr restarted. Called from processQueuedEntries.
func (m *Manager) resumeTorboxUsenetPolling(entry *storage.Entry, usenetID string) {
	defer m.processingEntries.Delete(entry.InfoHash)

	client := m.ProviderClient(entry.ActiveProvider)
	if client == nil {
		m.logger.Error().Str("debrid", entry.ActiveProvider).Msg("TorBox usenet resume: provider client not found")
		entry.MarkAsError(fmt.Errorf("debrid client not found: %s", entry.ActiveProvider))
		_ = m.queue.Update(entry)
		return
	}
	nc, ok := client.(debridCommon.NZBClient)
	if !ok {
		m.logger.Error().Str("debrid", entry.ActiveProvider).Msg("TorBox usenet resume: client does not implement NZBClient")
		entry.MarkAsError(fmt.Errorf("client %s does not support NZB", entry.ActiveProvider))
		_ = m.queue.Update(entry)
		return
	}

	m.logger.Info().Str("name", entry.Name).Str("usenet_id", usenetID).Msg("Resuming TorBox usenet poll after restart")

	bgCtx, cancel := context.WithTimeout(context.Background(), m.usenetTimeout)
	defer cancel()

	dl, err := nc.WaitForUsenetCached(bgCtx, usenetID, m.usenetTimeout, func(progress float64) {
		entry.Progress = progress
		entry.UpdatedAt = time.Now()
		_ = m.queue.Update(entry)
	})
	if err != nil {
		m.logger.Warn().Err(err).Str("name", entry.Name).Str("usenet_id", usenetID).
			Msg("TorBox usenet cache wait failed after restart — marking entry errored")
		_ = nc.DeleteUsenetDownload(bgCtx, usenetID)
		entry.State = storage.EntryStateError
		entry.Status = debridTypes.TorrentStatusError
		entry.UpdatedAt = time.Now()
		_ = m.queue.Update(entry)
		return
	}

	m.finalizeTorboxUsenetEntry(entry, dl, usenetID, entry.ActiveProvider)
}

// finalizeTorboxUsenetEntry populates files and triggers processAction after a successful
// WaitForUsenetCached — shared by addNZBViaTorbox's goroutine and resumeTorboxUsenetPolling.
func (m *Manager) finalizeTorboxUsenetEntry(entry *storage.Entry, dl *debridCommon.UsenetDownload, usenetID, debridName string) {
	now := time.Now()
	providerEntry := entry.Providers[debridName]
	if providerEntry == nil {
		providerEntry = &storage.ProviderEntry{
			Provider: debridName,
			ID:       usenetID,
			AddedAt:  entry.AddedOn,
			Files:    make(map[string]*storage.ProviderFile),
		}
		entry.Providers[debridName] = providerEntry
	}
	providerEntry.DownloadedAt = &now
	providerEntry.Status = debridTypes.TorrentStatusDownloaded
	providerEntry.Progress = 1.0

	for _, f := range dl.Files {
		fileName := filepath.Base(f.Name)
		m.logger.Debug().Str("file", fileName).Int64("size", f.Size).Msg("TorBox usenet: file in download")
		link := torbox.BuildUsenetLink(usenetID, f.ID)
		providerEntry.Files[fileName] = &storage.ProviderFile{
			Id:   f.ID,
			Link: link,
			Path: f.Path,
		}
		entry.Files[fileName] = &storage.File{
			Name:     fileName,
			Size:     f.Size,
			InfoHash: entry.InfoHash,
			AddedOn:  entry.AddedOn,
			Path:     f.Path,
		}
	}

	if len(entry.Files) == 0 {
		m.logger.Warn().Str("name", entry.Name).Str("usenet_id", usenetID).
			Msg("TorBox usenet: no files after completion — marking entry errored")
		entry.State = storage.EntryStateError
		entry.Status = debridTypes.TorrentStatusError
		entry.UpdatedAt = time.Now()
		_ = m.queue.Update(entry)
		return
	}

	entry.Name = dl.Name
	entry.OriginalFilename = dl.Name
	entry.Size = dl.Size
	entry.Bytes = dl.Size
	entry.Status = debridTypes.TorrentStatusDownloaded
	entry.State = storage.EntryStatePausedUP
	entry.Progress = 1.0
	entry.UpdatedAt = now
	_ = m.queue.Update(entry)

	m.logger.Info().
		Str("name", dl.Name).
		Str("usenet_id", usenetID).
		Int("files", len(entry.Files)).
		Msg("TorBox usenet entry ready, processing action")

	go m.processAction(entry)
}

func (m *Manager) syncNZBs(ctx context.Context) error {
	if m.usenet == nil {
		return nil
	}

	newNZBs, err := m.usenet.ProcessNewNZBs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get new NZBs from usenet client: %w", err)
	}

	for _, meta := range newNZBs {
		// Skip if already in storage or queue to avoid overwriting in-progress entries
		if _, err := m.GetEntry(meta.ID); err == nil {
			continue
		}
		if _, err := m.queue.GetTorrent(meta.ID); err == nil {
			continue
		}

		entry := &storage.Entry{
			InfoHash:         meta.ID,
			Name:             meta.Name,
			OriginalFilename: meta.Name,
			Size:             meta.TotalSize,
			Protocol:         config.ProtocolNZB,
			Bytes:            meta.TotalSize,
			Category:         meta.Category,
			Status:           debridTypes.TorrentStatusDownloading,
			State:            storage.EntryStateDownloading,
			Progress:         0,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
			AddedOn:          time.Now(),
			Providers:        make(map[string]*storage.ProviderEntry),
			Files:            make(map[string]*storage.File),
			Tags:             []string{},
		}
		entry.ContentPath = entry.DownloadPath()

		// AddOrUpdate placement
		_ = entry.AddUsenetProvider(meta)
		entry.ActiveProvider = "usenet"
		// AddOrUpdate files here using logical streamable files
		for _, file := range meta.Files {
			tFile := &storage.File{
				Name:     file.Name,
				Size:     file.Size,
				InfoHash: entry.InfoHash,
				AddedOn:  entry.AddedOn,
				Path:     file.Name,
			}
			entry.Files[file.Name] = tFile
		}

		// Add the entry to storage
		if err := m.storage.AddOrUpdate(entry); err != nil {
			m.logger.Error().Err(err).Str("name", entry.Name).Msg("Failed to addOrUpdate synced NZB entry to storage")
			continue
		}
	}
	return nil
}
