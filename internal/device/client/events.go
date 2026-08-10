package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

// PushEvent is one metadata mutation to append on the coordinator (no file bytes).
type PushEvent struct {
	EventID     string     `json:"event_id"`
	Op          string     `json:"op"`
	Path        string     `json:"path"`
	OldPath     string     `json:"old_path,omitempty"`
	Size        int64      `json:"size"`
	ContentHash string     `json:"content_hash"`
	ModTime     *time.Time `json:"mtime,omitempty"`
	HLC         model.HLC  `json:"hlc"`
}

// PushEventsResult is the coordinator response after accepting a push batch.
type PushEventsResult struct {
	Accepted []model.FolderEvent `json:"accepted"`
	MaxSeq   int64               `json:"max_seq"`
}

// PullEventsResult is a page of events after a given sequence cursor.
type PullEventsResult struct {
	Events  []model.FolderEvent `json:"events"`
	MaxSeq  int64               `json:"max_seq"`
	HasMore bool                `json:"has_more"`
}

// PushFolderEvents POSTs local outbox events to the coordinator event log.
func (c *Client) PushFolderEvents(ctx context.Context, folderID string, events []PushEvent) (PushEventsResult, error) {
	if folderID == "" {
		return PushEventsResult{}, fmt.Errorf("folder id required")
	}
	if len(events) == 0 {
		return PushEventsResult{}, fmt.Errorf("events required")
	}
	path := "/v1/folders/" + url.PathEscape(folderID) + "/events"
	var out PushEventsResult
	if err := c.do(ctx, http.MethodPost, path, map[string]any{"events": events}, &out); err != nil {
		return PushEventsResult{}, err
	}
	return out, nil
}

// PullFolderEvents GETs events with seq > since for folderID.
func (c *Client) PullFolderEvents(ctx context.Context, folderID string, since int64, limit int) (PullEventsResult, error) {
	if folderID == "" {
		return PullEventsResult{}, fmt.Errorf("folder id required")
	}
	if limit <= 0 {
		limit = 200
	}
	q := url.Values{}
	q.Set("since", strconv.FormatInt(since, 10))
	q.Set("limit", strconv.Itoa(limit))
	path := "/v1/folders/" + url.PathEscape(folderID) + "/events?" + q.Encode()
	var out PullEventsResult
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return PullEventsResult{}, err
	}
	if out.Events == nil {
		out.Events = []model.FolderEvent{}
	}
	return out, nil
}
