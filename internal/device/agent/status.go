package agent

import (
	"context"
	"fmt"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/index"
	"github.com/Not-Satya/sync_engine/internal/device/transfer"
)

// FolderReport is a local binding plus live index / outbox / missing-blob stats.
type FolderReport struct {
	Row        bindings.StatusRow
	Alive      int
	Tombstones int
	Outbox     int
	Cursor     int64
	Missing    int // index entries whose bytes are absent or hash-mismatched locally
}

func (r FolderReport) String() string {
	return fmt.Sprintf("%s  files=%d  tombstones=%d  outbox=%d  cursor=%d  missing=%d",
		r.Row.String(), r.Alive, r.Tombstones, r.Outbox, r.Cursor, r.Missing)
}

// CollectFolderReports joins binding path health with index counts and
// missing-blob counts (local only; no network).
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
	planner := transfer.Planner{Index: idx}
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
		if row.Health == bindings.PathOK {
			missing, err := planner.Missing(ctx, row.Binding.FolderID, row.Binding.LocalPath)
			if err != nil {
				return nil, err
			}
			rep.Missing = len(missing)
		}
		out = append(out, rep)
	}
	return out, nil
}
