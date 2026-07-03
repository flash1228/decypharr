package torbox

import "time"

type usenetFileInfo struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	S3Path    string `json:"s3_path"`
	MimeType  string `json:"mimetype"`
	ShortName string `json:"short_name"`
}

type usenetInfo struct {
	Id               int              `json:"id"`
	AuthId           string           `json:"auth_id"`
	Server           int              `json:"server"`
	Hash             string           `json:"hash"`
	Name             string           `json:"name"`
	Size             int64            `json:"size"`
	Active           bool             `json:"active"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DownloadState    string           `json:"download_state"`
	Progress         float64          `json:"progress"`
	DownloadSpeed    int64            `json:"download_speed"`
	Eta              int              `json:"eta"`
	Files            []usenetFileInfo `json:"files"`
	DownloadPresent  bool             `json:"download_present"`
	DownloadFinished bool             `json:"download_finished"`
	Cached           bool             `json:"cached"`
}

type usenetListResponse APIResponse[[]usenetInfo]
type usenetInfoResponse APIResponse[usenetInfo]
type addNZBResponse APIResponse[struct {
	Id   int    `json:"usenet_id"`
	Hash string `json:"hash"`
}]
