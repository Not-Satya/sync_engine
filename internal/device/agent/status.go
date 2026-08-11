package agent

import (
	"context"
	"fmt"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

// FolderReport is a local binding plus live index / outbox stats for CLI status.
type FolderReport struct {
	Row        bindings.StatusRow
	Alive      int
	Tombstones int
	Outbox     int
	Cursor     int64
}

func (r FolderReport) String() string {
	return fmt.Sprintf("%s  files=%d  tombstones=%d  outbox=%d  cursor=%d",
		r.Row.String(), r.Alive, r.Tombstones, r.Outbox, r.Cursor)
}

// CollectFolderReports joins binding path health with index counts (no network).
func CollectFolderReports(ctx context.Context, bind *bindings.Store, idx *index.Store) ([]FolderReport, error) {
	if bind == nil {
		return nil, fmt.Errorf("agent: nil bindings")
	}
	if idx == nil {
		return nil, fmt.Errorf("agent: nil index")
	}
	rows, err := bind.Status()
	if err != nil {
		return nil, err
	}
	out := make([]FolderReport, 0, len(rows))
	for _, row := range rows {
		rep := FolderReport{Row: row}
		alive, tombs, err := idx.Count(ctx, row.Binding.FolderID)
		if err != nil {
			return nil, err
		}
		n, err := idx.OutboxCount(ctx, row.Binding.FolderID)
		if err != nil {
			return nil, err
		}
		cur, err := idx.Cursor(ctx, row.Binding.FolderID)
		if err != nil {
			return nil, err
		}
		rep.Alive = alive
		rep.Tombstones = tombs
		rep.Outbox = n
		rep.Cursor = cur
		out = append(out, rep)
	}
	return out, nil
}
