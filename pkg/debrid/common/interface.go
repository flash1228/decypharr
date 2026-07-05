package common

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/sirrobot01/decypharr/internal/config"
	"github.com/sirrobot01/decypharr/pkg/debrid/account"
	"github.com/sirrobot01/decypharr/pkg/debrid/types"
)

type Client interface {
	SubmitMagnet(tr *types.Torrent) (*types.Torrent, error)
	CheckStatus(tr *types.Torrent) (*types.Torrent, error)
	GetDownloadLink(torrentID string, file *types.File) (types.DownloadLink, error)
	DeleteTorrent(torrentId string) error
	IsAvailable(infohashes []string) map[string]bool
	UpdateTorrent(torrent *types.Torrent) error
	GetTorrent(torrentId string) (*types.Torrent, error)
	GetTorrents() ([]*types.Torrent, error)
	Config() config.Debrid
	Logger() zerolog.Logger
	RefreshDownloadLinks() error
	CheckFile(ctx context.Context, infohash, fileID string) error // fileID here can link, file id(in the case of torbox), etc.
	AccountManager() *account.Manager                             // Returns the active download account/token
	GetProfile() (*types.Profile, error)
	GetAvailableSlots() (int, error)
	SyncAccounts() // Updates each accounts details(like traffic, username, etc.)
	DeleteLink(dl types.DownloadLink) error
	SpeedTest(ctx context.Context) types.SpeedTestResult
	SupportsCheck() bool
}

// UsenetDownload describes a completed usenet download on a debrid service.
type UsenetDownload struct {
	ID    string
	Name  string
	Size  int64
	Files []UsenetFile
}

// UsenetFile describes a single file within a debrid usenet download.
type UsenetFile struct {
	ID   string
	Name string
	Size int64
	Path string
}

// NZBClient is an optional capability for debrid providers that support
// direct NZB submission (e.g. TorBox Pro usenet API). Check for it with
// a type assertion before use — not all providers implement this.
type NZBClient interface {
	// SupportsUsenet returns true only when the account's plan allows usenet API access.
	SupportsUsenet() bool
	// SubmitNZB uploads NZB content and returns the provider's usenet download ID.
	SubmitNZB(ctx context.Context, nzbContent []byte, name string) (string, error)
	// WaitForUsenetCached polls until the download is cached/finished or ctx is cancelled.
	// onProgress is called on each poll with current progress (0.0–1.0); may be nil.
	WaitForUsenetCached(ctx context.Context, id string, onProgress func(float64)) (*UsenetDownload, error)
	// DeleteUsenetDownload removes a usenet download from the provider.
	DeleteUsenetDownload(ctx context.Context, id string) error
	// GetActiveUsenetCount returns how many usenet downloads are currently active (queued or downloading).
	GetActiveUsenetCount(ctx context.Context) (int, error)
}
