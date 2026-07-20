package store

import (
	"encoding/json"
	"errors"
	"testing"

	"obdurate/internal/model"
)

func decodeData(t *testing.T, a model.Activity) map[string]any {
	t.Helper()
	if len(a.Data) == 0 {
		t.Fatalf("activity %d (%s) has no data payload", a.ID, a.Kind)
	}
	var m map[string]any
	if err := json.Unmarshal(a.Data, &m); err != nil {
		t.Fatalf("unmarshal data %q: %v", string(a.Data), err)
	}
	return m
}

func findActivity(t *testing.T, list []model.Activity, kind string) model.Activity {
	t.Helper()
	for _, a := range list {
		if a.Kind == kind {
			return a
		}
	}
	t.Fatalf("no activity of kind %q in %d entries", kind, len(list))
	return model.Activity{}
}

func TestCreateTaskDefaultsAndActivity(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "  First  ", ActorRef: "alice"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.Title != "First" {
		t.Errorf("title = %q, want trimmed %q", task.Title, "First")
	}
	if task.ColumnName != "Todo" || task.Priority != model.PriorityMedium || task.Position != 0 {
		t.Errorf("defaults wrong: column=%q priority=%q position=%d", task.ColumnName, task.Priority, task.Position)
	}
	second, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "Second"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.Position != 1 {
		t.Errorf("second position = %d, want 1", second.Position)
	}

	acts, err := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	created := findActivity(t, acts, model.ActivityCreated)
	if created.ActorRef != "alice" {
		t.Errorf("actor = %q, want alice", created.ActorRef)
	}
	snap, ok := decodeData(t, created)["task"].(map[string]any)
	if !ok {
		t.Fatalf("created payload missing task snapshot: %v", decodeData(t, created))
	}
	if snap["title"] != "First" || snap["column"] != "Todo" || snap["priority"] != "medium" {
		t.Errorf("snapshot = %v", snap)
	}
}

func TestCreateTaskValidation(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "  "}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty title: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "x", Priority: "urgent"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad priority: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/nope", Title: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad board: err = %v, want ErrNotFound", err)
	}
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "x", AssigneeRef: "ghost"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad assignee: err = %v, want ErrNotFound", err)
	}
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "x", ColumnRef: "Nope"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad column: err = %v, want ErrNotFound", err)
	}
}

func TestCreateTaskTagsAndWatchers(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{
		BoardRef:    "p1/b1",
		Title:       "tagged",
		Tags:        []string{" UI ", "ui", "auth", ""},
		WatcherRefs: []string{"bob@example.com", "BOB", "alice"},
		AssigneeRef: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(task.Tags) != 2 {
		t.Errorf("tags = %v, want 2 deduped", task.Tags)
	}
	if len(task.WatcherRefs) != 2 {
		t.Errorf("watchers = %v, want [alice bob]", task.WatcherRefs)
	}
	if task.AssigneeRef != "alice" {
		t.Errorf("assignee = %q, want alice", task.AssigneeRef)
	}
}

func TestUpdateTaskChangesPayload(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "orig", AssigneeRef: "alice", Tags: []string{"a"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := f.s.UpdateTask(task.ID, TaskUpdate{
		Title:       strP("renamed"),
		Priority:    prioP(model.PriorityHigh),
		AssigneeRef: strP("bob"),
		Tags:        &[]string{"a", "b"},
		ActorRef:    "bob",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "renamed" || updated.Priority != model.PriorityHigh || updated.AssigneeRef != "bob" {
		t.Errorf("updated = %+v", updated)
	}
	acts, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	up := findActivity(t, acts, model.ActivityUpdated)
	changes, ok := decodeData(t, up)["changes"].(map[string]any)
	if !ok {
		t.Fatalf("updated payload missing changes: %s", string(up.Data))
	}
	title := changes["title"].(map[string]any)
	if title["old"] != "orig" || title["new"] != "renamed" {
		t.Errorf("title change = %v", title)
	}
	assignee := changes["assignee"].(map[string]any)
	if assignee["old"] != "alice" || assignee["new"] != "bob" {
		t.Errorf("assignee change = %v", assignee)
	}
	if _, ok := changes["tags"]; !ok {
		t.Errorf("tags change missing: %v", changes)
	}

	// Clearing the assignee records old → null.
	if _, err := f.s.UpdateTask(task.ID, TaskUpdate{AssigneeRef: strP("")}); err != nil {
		t.Fatalf("clear assignee: %v", err)
	}
	got, _ := f.s.GetTask(task.ID)
	if got.AssigneeID != nil {
		t.Errorf("assignee not cleared: %+v", got)
	}

	// Empty title is rejected.
	if _, err := f.s.UpdateTask(task.ID, TaskUpdate{Title: strP(" ")}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty title: err = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateTaskNoopTagsLogsNothing(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "t", Tags: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	before, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	// Same tag set, different order and case: no change, no activity.
	if _, err := f.s.UpdateTask(task.ID, TaskUpdate{Tags: &[]string{"B", "A"}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if len(after) != len(before) {
		t.Errorf("activity grew from %d to %d on no-op tag update", len(before), len(after))
	}
}

func TestMoveTaskCompactionAndClamp(t *testing.T) {
	f := newFixture(t)
	var ids []int64
	for _, title := range []string{"t0", "t1", "t2"} {
		task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: title})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		ids = append(ids, task.ID)
	}
	// Move the middle task to another column; source compacts.
	moved, err := f.s.MoveTask(ids[1], "Doing", nil, "alice")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.ColumnName != "Doing" || moved.Position != 0 {
		t.Errorf("moved = column %q pos %d, want Doing 0", moved.ColumnName, moved.Position)
	}
	t0, _ := f.s.GetTask(ids[0])
	t2, _ := f.s.GetTask(ids[2])
	if t0.Position != 0 || t2.Position != 1 {
		t.Errorf("source not compacted: t0=%d t2=%d, want 0 and 1", t0.Position, t2.Position)
	}
	// Payload has from/to.
	acts, _ := f.s.ListActivity(ActivityFilter{TaskID: ids[1]})
	mv := findActivity(t, acts, model.ActivityMoved)
	data := decodeData(t, mv)
	from := data["from"].(map[string]any)
	to := data["to"].(map[string]any)
	if from["column"] != "Todo" || to["column"] != "Doing" {
		t.Errorf("move payload = %v", data)
	}
	// Negative explicit position clamps to 0 and shifts the occupant.
	if _, err := f.s.MoveTask(ids[2], "Todo", intPtr(-9), ""); err != nil {
		t.Fatalf("move clamp negative: %v", err)
	}
	t2, _ = f.s.GetTask(ids[2])
	t0, _ = f.s.GetTask(ids[0])
	if t2.Position != 0 || t0.Position != 1 {
		t.Errorf("after clamped move: t2=%d t0=%d, want 0 and 1", t2.Position, t0.Position)
	}
	// Past-the-end clamps to append.
	if _, err := f.s.MoveTask(ids[2], "Todo", intPtr(50), ""); err != nil {
		t.Fatalf("move clamp big: %v", err)
	}
	t2, _ = f.s.GetTask(ids[2])
	if t2.Position != 1 {
		t.Errorf("past-end position = %d, want 1 (append)", t2.Position)
	}
}

func TestDeleteTaskPreservesHistory(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "doomed", ActorRef: "alice"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.s.UpdateTask(task.ID, TaskUpdate{Title: strP("doomed-v2"), ActorRef: "alice"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := f.s.CommentTask(task.ID, "bob", "so long"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if err := f.s.DeleteTask(task.ID, "alice"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.s.GetTask(task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("task still exists: err = %v", err)
	}

	acts, err := f.s.ListActivity(ActivityFilter{BoardRef: "p1/b1"})
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	for _, kind := range []string{model.ActivityCreated, model.ActivityUpdated, model.ActivityCommented, model.ActivityDeleted} {
		a := findActivity(t, acts, kind)
		if a.TaskID != nil {
			t.Errorf("%s activity still references deleted task id %d", kind, *a.TaskID)
		}
	}
	// Detached rows keep the original id in data.task_id.
	created := findActivity(t, acts, model.ActivityCreated)
	if got := decodeData(t, created)["task_id"]; got != float64(task.ID) {
		t.Errorf("created data.task_id = %v, want %d", got, task.ID)
	}
	// The deleted entry snapshots the final state.
	deleted := findActivity(t, acts, model.ActivityDeleted)
	snap := decodeData(t, deleted)["task"].(map[string]any)
	if snap["title"] != "doomed-v2" {
		t.Errorf("deleted snapshot title = %v, want doomed-v2", snap["title"])
	}
}

func TestWatchUnwatchIdempotent(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "w"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := f.s.WatchTask(task.ID, "bob"); err != nil {
			t.Fatalf("watch #%d: %v", i+1, err)
		}
	}
	got, _ := f.s.GetTask(task.ID)
	if len(got.WatcherRefs) != 1 || got.WatcherRefs[0] != "bob" {
		t.Errorf("watchers = %v, want [bob]", got.WatcherRefs)
	}
	acts, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	watched := 0
	for _, a := range acts {
		if a.Kind == model.ActivityWatched {
			watched++
		}
	}
	if watched != 1 {
		t.Errorf("watched activities = %d, want 1 (idempotent)", watched)
	}
	if err := f.s.UnwatchTask(task.ID, "bob"); err != nil {
		t.Fatalf("unwatch: %v", err)
	}
	got, _ = f.s.GetTask(task.ID)
	if len(got.WatcherRefs) != 0 {
		t.Errorf("watchers after unwatch = %v", got.WatcherRefs)
	}
}

func TestListTasksFilters(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateBoard("p1", "b2", "", ""); err != nil {
		t.Fatalf("create b2: %v", err)
	}
	mk := func(board, title, assignee, tag string, watchers ...string) {
		t.Helper()
		in := TaskCreate{BoardRef: board, Title: title, AssigneeRef: assignee, WatcherRefs: watchers}
		if tag != "" {
			in.Tags = []string{tag}
		}
		if _, err := f.s.CreateTask(in); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	mk("p1/b1", "a1", "alice", "bug", "bob")
	mk("p1/b1", "a2", "bob", "feature")
	mk("p1/b2", "a3", "alice", "bug")

	cases := []struct {
		name string
		f    TaskFilter
		want int
	}{
		{"by board", TaskFilter{BoardRef: "p1/b1"}, 2},
		{"by project", TaskFilter{ProjectRef: "p1"}, 3},
		{"by assignee", TaskFilter{AssigneeRef: "alice"}, 2},
		{"by tag", TaskFilter{Tag: "BUG"}, 2},
		{"by watcher", TaskFilter{WatcherRef: "bob"}, 1},
		{"board+column", TaskFilter{BoardRef: "p1/b1", ColumnRef: "Todo"}, 2},
		{"board+empty column", TaskFilter{BoardRef: "p1/b1", ColumnRef: "Done"}, 0},
		{"no filter", TaskFilter{}, 3},
	}
	for _, c := range cases {
		got, err := f.s.ListTasks(c.f)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got == nil {
			t.Errorf("%s: nil slice", c.name)
		}
		if len(got) != c.want {
			t.Errorf("%s: %d tasks, want %d", c.name, len(got), c.want)
		}
	}
}

func TestBoardView(t *testing.T) {
	f := newFixture(t)
	if _, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "t", ColumnRef: "Doing"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	view, err := f.s.BoardView("p1/b1")
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if len(view.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(view.Columns))
	}
	for _, c := range view.Columns {
		if c.Tasks == nil {
			t.Errorf("column %q has nil Tasks; must be [] for JSON", c.Column.Name)
		}
	}
	if len(view.Columns[1].Tasks) != 1 {
		t.Errorf("Doing tasks = %d, want 1", len(view.Columns[1].Tasks))
	}
}

func TestTaskMetadataCRUD(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "m"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(task.Metadata) != 0 {
		t.Errorf("initial metadata = %v, want empty", task.Metadata)
	}

	updated, err := f.s.SetTaskMetadata(task.ID, "  Jira-Key ", "PROJ-123", "alice")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if updated.Metadata["jira-key"] != "PROJ-123" {
		t.Errorf("metadata = %v, want jira-key=PROJ-123 (key lowercased)", updated.Metadata)
	}

	value, err := f.s.GetTaskMetadata(task.ID, "JIRA-KEY")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if value != "PROJ-123" {
		t.Errorf("get value = %q, want PROJ-123", value)
	}

	acts, err := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	set := findActivity(t, acts, model.ActivityUpdated)
	changes, ok := decodeData(t, set)["changes"].(map[string]any)
	if !ok {
		t.Fatalf("set payload missing changes: %s", string(set.Data))
	}
	change, ok := changes["metadata.jira-key"].(map[string]any)
	if !ok {
		t.Fatalf("changes missing metadata.jira-key: %v", changes)
	}
	if change["old"] != nil || change["new"] != "PROJ-123" {
		t.Errorf("metadata change = %v, want old=nil new=PROJ-123", change)
	}

	// Setting the same value again is a no-op: no new activity.
	before := len(acts)
	if _, err := f.s.SetTaskMetadata(task.ID, "jira-key", "PROJ-123", "alice"); err != nil {
		t.Fatalf("no-op set: %v", err)
	}
	after, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if len(after) != before {
		t.Errorf("activity grew from %d to %d on no-op metadata set", before, len(after))
	}

	// Overwriting logs old -> new.
	if _, err := f.s.SetTaskMetadata(task.ID, "jira-key", "PROJ-456", ""); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	acts, _ = f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	var overwrite model.Activity
	for _, a := range acts {
		if a.Kind != model.ActivityUpdated {
			continue
		}
		c, _ := decodeData(t, a)["changes"].(map[string]any)
		if ch, ok := c["metadata.jira-key"].(map[string]any); ok && ch["new"] == "PROJ-456" {
			overwrite = a
			break
		}
	}
	if overwrite.ID == 0 {
		t.Fatalf("no activity found for metadata overwrite")
	}
	ch := decodeData(t, overwrite)["changes"].(map[string]any)["metadata.jira-key"].(map[string]any)
	if ch["old"] != "PROJ-123" || ch["new"] != "PROJ-456" {
		t.Errorf("overwrite change = %v, want old=PROJ-123 new=PROJ-456", ch)
	}

	// Delete removes the key and logs old -> nil; deleting again is a no-op.
	deleted, err := f.s.DeleteTaskMetadata(task.ID, "jira-key", "bob")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := deleted.Metadata["jira-key"]; ok {
		t.Errorf("metadata still has jira-key after delete: %v", deleted.Metadata)
	}
	if _, err := f.s.GetTaskMetadata(task.ID, "jira-key"); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
	beforeDel, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if _, err := f.s.DeleteTaskMetadata(task.ID, "jira-key", "bob"); err != nil {
		t.Fatalf("no-op delete: %v", err)
	}
	afterDel, _ := f.s.ListActivity(ActivityFilter{TaskID: task.ID})
	if len(afterDel) != len(beforeDel) {
		t.Errorf("activity grew from %d to %d on no-op delete", len(beforeDel), len(afterDel))
	}
}

func TestTaskMetadataInvalidKey(t *testing.T) {
	f := newFixture(t)
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "m"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.s.SetTaskMetadata(task.ID, "", "v", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty key: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.SetTaskMetadata(task.ID, "not a slug", "v", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("invalid key: err = %v, want ErrInvalidInput", err)
	}
	if _, err := f.s.SetTaskMetadata(999999, "k", "v", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bad task id: err = %v, want ErrNotFound", err)
	}
}

func TestListActivityLimitAndEmptyNonNil(t *testing.T) {
	f := newFixture(t)
	list, err := f.s.ListActivity(ActivityFilter{TaskID: 12345})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list == nil {
		t.Error("ListActivity returned nil slice; must be empty non-nil for JSON []")
	}
	task, err := f.s.CreateTask(TaskCreate{BoardRef: "p1/b1", Title: "busy"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.s.CommentTask(task.ID, "alice", "c"); err != nil {
			t.Fatalf("comment: %v", err)
		}
	}
	list, err = f.s.ListActivity(ActivityFilter{TaskID: task.ID, Limit: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("limited list = %d, want 3", len(list))
	}
}
