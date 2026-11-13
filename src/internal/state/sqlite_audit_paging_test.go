package state

import (
	"path/filepath"
	"testing"
)

func TestAuditPagingAndSorting(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "paging.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.InitCatalog(); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{"alice", "bob", "charlie"} {
		if err = db.Audit(actor, "service.update", "ok"); err != nil {
			t.Fatal(err)
		}
	}
	events, total, err := db.PageAudit(1, 2, "actor", "asc")
	if err != nil || total != 3 || len(events) != 2 || events[0].Actor != "alice" || events[1].Actor != "bob" {
		t.Fatalf("page 1 %v %d %v", events, total, err)
	}
	events, total, err = db.PageAudit(2, 2, "actor", "asc")
	if err != nil || total != 3 || len(events) != 1 || events[0].Actor != "charlie" {
		t.Fatalf("page 2 %v %d %v", events, total, err)
	}
	events, _, err = db.PageAudit(1, 3, "actor;DROP TABLE audit", "asc")
	if err != nil || len(events) != 3 {
		t.Fatalf("unsafe sort was not normalized: %v", err)
	}
	if _, _, err = db.PageAudit(1, 500, "time", "desc"); err == nil {
		t.Fatal("unbounded audit pagination")
	}
}
