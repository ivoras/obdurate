package store

import (
	"testing"

	"obdurate/internal/model"
)

// findEntityActivity returns the newest activity row of the given kind whose
// data payload names the given entity.
func findEntityActivity(t *testing.T, list []model.Activity, kind, entity string) model.Activity {
	t.Helper()
	for _, a := range list {
		if a.Kind != kind || len(a.Data) == 0 {
			continue
		}
		if decodeData(t, a)["entity"] == entity {
			return a
		}
	}
	t.Fatalf("no %q activity for entity %q in %d entries", kind, entity, len(list))
	return model.Activity{}
}

func TestProjectLifecycleLogged(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateDeveloper("Alice", "alice@example.com", "alice", nil, model.RoleLead); err != nil {
		t.Fatalf("create dev: %v", err)
	}
	p, err := s.CreateProject("p1", "desc", "alice")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	acts, err := s.ListActivity(ActivityFilter{ProjectRef: "p1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	created := findEntityActivity(t, acts, model.ActivityCreated, "project")
	if created.ActorRef != "alice" {
		t.Errorf("actor = %q, want alice", created.ActorRef)
	}
	snap := decodeData(t, created)["project"].(map[string]any)
	if snap["name"] != "p1" || snap["description"] != "desc" {
		t.Errorf("project snapshot = %v", snap)
	}

	if _, err := s.UpdateProject("p1", ProjectUpdate{Name: strP("p1x"), ActorRef: "alice"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	acts, _ = s.ListActivity(ActivityFilter{ProjectRef: "p1x"})
	up := findEntityActivity(t, acts, model.ActivityUpdated, "project")
	name := decodeData(t, up)["changes"].(map[string]any)["name"].(map[string]any)
	if name["old"] != "p1" || name["new"] != "p1x" {
		t.Errorf("name change = %v", name)
	}
	// No-op update logs nothing.
	n := len(acts)
	if _, err := s.UpdateProject("p1x", ProjectUpdate{Name: strP("P1X")}); err != nil {
		t.Fatalf("noop update: %v", err)
	}
	acts, _ = s.ListActivity(ActivityFilter{ProjectRef: "p1x"})
	if len(acts) != n {
		t.Errorf("no-op update logged activity: %d → %d entries", n, len(acts))
	}

	if err := s.DeleteProject("p1x", "alice"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	global, err := s.ListActivity(ActivityFilter{})
	if err != nil {
		t.Fatalf("list global: %v", err)
	}
	del := findEntityActivity(t, global, model.ActivityDeleted, "project")
	if decodeData(t, del)["project"].(map[string]any)["name"] != "p1x" {
		t.Errorf("deleted snapshot = %s", string(del.Data))
	}
	// The project's earlier history is preserved, detached with the id in data.
	createdAfter := findEntityActivity(t, global, model.ActivityCreated, "project")
	if createdAfter.ProjectID != nil {
		t.Errorf("created row still references deleted project %d", *createdAfter.ProjectID)
	}
	if got := decodeData(t, createdAfter)["project_id"]; got != float64(p.ID) {
		t.Errorf("created data.project_id = %v, want %d", got, p.ID)
	}
}

func TestBoardAndColumnLifecycleLogged(t *testing.T) {
	f := newFixture(t)
	acts, _ := f.s.ListActivity(ActivityFilter{BoardRef: "p1/b1"})
	bc := findEntityActivity(t, acts, model.ActivityCreated, "board")
	if decodeData(t, bc)["board"].(map[string]any)["name"] != "b1" {
		t.Errorf("board created payload = %s", string(bc.Data))
	}

	if _, err := f.s.AddColumn("p1/b1", "Review", intPtr(2), "alice"); err != nil {
		t.Fatalf("add column: %v", err)
	}
	if _, err := f.s.RenameColumn("p1/b1", "Review", "QA", "alice"); err != nil {
		t.Fatalf("rename column: %v", err)
	}
	if _, err := f.s.ReorderColumn("p1/b1", "QA", 0, "alice"); err != nil {
		t.Fatalf("reorder column: %v", err)
	}
	if err := f.s.DeleteColumn("p1/b1", "QA", "alice"); err != nil {
		t.Fatalf("delete column: %v", err)
	}
	acts, _ = f.s.ListActivity(ActivityFilter{BoardRef: "p1/b1"})

	cc := findEntityActivity(t, acts, model.ActivityCreated, "column")
	if decodeData(t, cc)["column"].(map[string]any)["name"] != "Review" {
		t.Errorf("column created payload = %s", string(cc.Data))
	}
	cu := findEntityActivity(t, acts, model.ActivityUpdated, "column")
	rename := decodeData(t, cu)["changes"].(map[string]any)["name"].(map[string]any)
	if rename["old"] != "Review" || rename["new"] != "QA" {
		t.Errorf("rename change = %v", rename)
	}
	cm := findEntityActivity(t, acts, model.ActivityMoved, "column")
	cmData := decodeData(t, cm)
	if cmData["from"].(map[string]any)["position"] != float64(2) || cmData["to"].(map[string]any)["position"] != float64(0) {
		t.Errorf("column move payload = %s", string(cm.Data))
	}
	cd := findEntityActivity(t, acts, model.ActivityDeleted, "column")
	if cd.ActorRef != "alice" {
		t.Errorf("column delete actor = %q, want alice", cd.ActorRef)
	}

	// Board deletion keeps history in the project stream.
	if err := f.s.DeleteBoard("p1/b1", "alice"); err != nil {
		t.Fatalf("delete board: %v", err)
	}
	projActs, _ := f.s.ListActivity(ActivityFilter{ProjectRef: "p1"})
	bd := findEntityActivity(t, projActs, model.ActivityDeleted, "board")
	if decodeData(t, bd)["board"].(map[string]any)["name"] != "b1" {
		t.Errorf("board deleted payload = %s", string(bd.Data))
	}
	bcAfter := findEntityActivity(t, projActs, model.ActivityCreated, "board")
	if bcAfter.BoardID != nil {
		t.Errorf("board created row still references deleted board %d", *bcAfter.BoardID)
	}
	if got := decodeData(t, bcAfter)["board_id"]; got != float64(f.board.ID) {
		t.Errorf("board created data.board_id = %v, want %d", got, f.board.ID)
	}
}

func TestDeveloperLifecycleLogged(t *testing.T) {
	s := newTestStore(t)
	d, err := s.CreateDeveloper("Alice", "alice@example.com", "alice", nil, model.RoleLead)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acts, _ := s.ListActivity(ActivityFilter{})
	dc := findEntityActivity(t, acts, model.ActivityCreated, "developer")
	if decodeData(t, dc)["developer"].(map[string]any)["username"] != "alice" {
		t.Errorf("developer created payload = %s", string(dc.Data))
	}

	if _, err := s.UpdateDeveloper("alice", DeveloperUpdate{Email: strP("new@example.com")}); err != nil {
		t.Fatalf("update: %v", err)
	}
	acts, _ = s.ListActivity(ActivityFilter{})
	du := findEntityActivity(t, acts, model.ActivityUpdated, "developer")
	email := decodeData(t, du)["changes"].(map[string]any)["email"].(map[string]any)
	if email["old"] != "alice@example.com" || email["new"] != "new@example.com" {
		t.Errorf("email change = %v", email)
	}

	// Authorship survives developer deletion via data.actor.
	if _, err := s.CreateProject("p1", "", "alice"); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := s.DeleteDeveloper("alice"); err != nil {
		t.Fatalf("delete developer: %v", err)
	}
	acts, _ = s.ListActivity(ActivityFilter{})
	dd := findEntityActivity(t, acts, model.ActivityDeleted, "developer")
	if decodeData(t, dd)["developer"].(map[string]any)["id"] != float64(d.ID) {
		t.Errorf("developer deleted payload = %s", string(dd.Data))
	}
	pc := findEntityActivity(t, acts, model.ActivityCreated, "project")
	if pc.ActorRef != "" {
		t.Errorf("actor join = %q, want empty after developer delete", pc.ActorRef)
	}
	if got := decodeData(t, pc)["actor"]; got != "alice" {
		t.Errorf("preserved data.actor = %v, want alice", got)
	}
}
