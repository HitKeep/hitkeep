package worker

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/database"
)

func TestRetentionArchivesAndRestoresVisibleTimeWithItsPageview(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)
	mgr := newTestTenantMgr(t, store)
	siteID := seedSite(t, ctx, store, 1)
	pageID, retainedPageID := uuid.New(), uuid.New()
	old, now := time.Now().UTC().Add(-48*time.Hour), time.Now().UTC()
	for _, hit := range []api.Hit{
		{SiteID: siteID, SessionID: uuid.New(), PageID: pageID, Timestamp: old, Path: "/old"},
		{SiteID: siteID, SessionID: uuid.New(), PageID: retainedPageID, Timestamp: old, Path: "/duplicate"},
		{SiteID: siteID, SessionID: uuid.New(), PageID: retainedPageID, Timestamp: now, Path: "/kept"},
	} {
		if err := store.CreateHit(ctx, &hit); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mgr.ApplyPageVisibleTime(ctx, store, []api.PageVisibleTimeSample{
		{SiteID: siteID, PageID: pageID, VisibleMS: 3000},
		{SiteID: siteID, PageID: retainedPageID, VisibleMS: 870},
	}); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(t.TempDir(), "archive")
	if err := NewRetentionWorker(mgr, archiveDir, 365, nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	files, err := findParquetFiles(archiveDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("retention archive count=%d err=%v", len(files), err)
	}
	var ms int
	if err := store.DB().QueryRowContext(ctx, "SELECT visible_ms FROM page_visible_time WHERE page_id = ?", retainedPageID).Scan(&ms); err != nil || ms != 870 {
		t.Fatalf("sample for retained duplicate: ms=%d err=%v", ms, err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM page_visible_time WHERE page_id = ?", pageID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired sample count=%d err=%v", count, err)
	}

	tenantPath := filepath.Join(t.TempDir(), "restore.db")
	restored := database.NewStore(tenantPath)
	if err := restored.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if err := restored.MigrateTenant(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restored.UpsertSiteMirror(ctx, &api.Site{ID: siteID, Domain: "test.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		summary, err := database.ImportRetentionArchives(ctx, tenantPath, files)
		if err != nil || summary.Imported["page_visible_time"] != int64(1-i) {
			t.Fatalf("attempt %d restore summary=%+v err=%v", i+1, summary, err)
		}
	}
	reopened := database.NewStore(tenantPath)
	if err := reopened.Connect(); err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.DB().QueryRowContext(ctx, "SELECT visible_ms FROM page_visible_time WHERE site_id = ? AND page_id = ?", siteID, pageID).Scan(&ms); err != nil || ms != 3000 {
		t.Fatalf("restored sample ms=%d err=%v", ms, err)
	}
}
