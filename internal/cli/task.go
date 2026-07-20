package cli

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"obdurate/internal/model"
	"obdurate/internal/store"
)

func newTaskCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(
		taskCreate(app),
		taskList(app),
		taskGet(app),
		taskUpdate(app),
		taskMove(app),
		taskDelete(app),
		taskComment(app),
		taskWatch(app),
		taskUnwatch(app),
		taskActivity(app),
		taskMine(app),
		newTaskMetadataCmd(app),
	)
	return cmd
}

func taskCreate(app *App) *cobra.Command {
	var board, title, description, column, assignee, priority, tags, watchers, by string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task on a board",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := app.Store.CreateTask(store.TaskCreate{
				BoardRef:    board,
				ColumnRef:   column,
				Title:       title,
				Description: description,
				AssigneeRef: assignee,
				Priority:    model.Priority(priority),
				Tags:        splitCSVFlag(tags),
				WatcherRefs: splitCSVFlag(watchers),
				ActorRef:    by,
			})
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	cmd.Flags().StringVar(&title, "title", "", "title (required)")
	cmd.Flags().StringVar(&description, "description", "", "description")
	cmd.Flags().StringVar(&column, "column", "", "column name/id (default: first)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "developer ref")
	cmd.Flags().StringVar(&priority, "priority", "medium", "low|medium|high|critical")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags")
	cmd.Flags().StringVar(&watchers, "watchers", "", "comma-separated developer refs")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func taskList(app *App) *cobra.Command {
	var board, project, assignee, column, watcher, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks with optional filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListTasks(store.TaskFilter{
				BoardRef:    board,
				ProjectRef:  project,
				AssigneeRef: assignee,
				ColumnRef:   column,
				WatcherRef:  watcher,
				Tag:         tag,
			})
			if err != nil {
				return err
			}
			return printTaskList(app, list)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "filter by board")
	cmd.Flags().StringVar(&project, "project", "", "filter by project")
	cmd.Flags().StringVar(&assignee, "assignee", "", "filter by assignee ref")
	cmd.Flags().StringVar(&column, "column", "", "filter by column (with --board)")
	cmd.Flags().StringVar(&watcher, "watcher", "", "filter by watcher ref")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}

func taskGet(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get task by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			t, err := app.Store.GetTask(id)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
}

func taskUpdate(app *App) *cobra.Command {
	var title, description, assignee, priority, tags, by string
	var clearAssignee bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update task fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			u := store.TaskUpdate{ActorRef: by}
			if cmd.Flags().Changed("title") {
				u.Title = &title
			}
			if cmd.Flags().Changed("description") {
				u.Description = &description
			}
			if clearAssignee {
				empty := ""
				u.AssigneeRef = &empty
			} else if cmd.Flags().Changed("assignee") {
				u.AssigneeRef = &assignee
			}
			if cmd.Flags().Changed("priority") {
				p := model.Priority(priority)
				u.Priority = &p
			}
			if cmd.Flags().Changed("tags") {
				t := splitCSVFlag(tags)
				u.Tags = &t
			}
			t, err := app.Store.UpdateTask(id, u)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee ref")
	cmd.Flags().BoolVar(&clearAssignee, "clear-assignee", false, "unassign task")
	cmd.Flags().StringVar(&priority, "priority", "", "low|medium|high|critical")
	cmd.Flags().StringVar(&tags, "tags", "", "replace tags (comma-separated)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	return cmd
}

func taskMove(app *App) *cobra.Command {
	var column, by string
	var position int
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Move task to another column",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			var pos *int
			if cmd.Flags().Changed("position") {
				pos = &position
			}
			t, err := app.Store.MoveTask(id, column, pos, by)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&column, "column", "", "target column name/id (required)")
	cmd.Flags().IntVar(&position, "position", 0, "position within column")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	_ = cmd.MarkFlagRequired("column")
	return cmd
}

func taskDelete(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			if err := app.Store.DeleteTask(id, by); err != nil {
				return err
			}
			app.Print.PrintOK(fmt.Sprintf("deleted task %d", id))
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(map[string]any{"status": "deleted", "id": id})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	return cmd
}

func taskComment(app *App) *cobra.Command {
	var message, by string
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Add a comment to the task activity stream",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			a, err := app.Store.CommentTask(id, by, message)
			if err != nil {
				return err
			}
			return printActivityOne(app, a)
		},
	}
	cmd.Flags().StringVar(&message, "message", "", "comment text (required)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func taskWatch(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "watch <id>",
		Short: "Add a watcher to a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			if by == "" {
				return fmt.Errorf("%w: --by is required", store.ErrInvalidInput)
			}
			if err := app.Store.WatchTask(id, by); err != nil {
				return err
			}
			t, err := app.Store.GetTask(id)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "developer ref (required)")
	_ = cmd.MarkFlagRequired("by")
	return cmd
}

func taskUnwatch(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "unwatch <id>",
		Short: "Remove a watcher from a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			if by == "" {
				return fmt.Errorf("%w: --by is required", store.ErrInvalidInput)
			}
			if err := app.Store.UnwatchTask(id, by); err != nil {
				return err
			}
			t, err := app.Store.GetTask(id)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "developer ref (required)")
	_ = cmd.MarkFlagRequired("by")
	return cmd
}

func taskActivity(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "activity <id>",
		Short: "Show activity/comments for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			list, err := app.Store.ListActivity(store.ActivityFilter{TaskID: id, Limit: limit})
			if err != nil {
				return err
			}
			return printActivityList(app, list)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max entries")
	return cmd
}

func taskMine(app *App) *cobra.Command {
	var assignee string
	cmd := &cobra.Command{
		Use:   "mine",
		Short: "List tasks assigned to a developer",
		RunE: func(cmd *cobra.Command, args []string) error {
			if assignee == "" {
				return fmt.Errorf("%w: --assignee is required", store.ErrInvalidInput)
			}
			list, err := app.Store.ListTasks(store.TaskFilter{AssigneeRef: assignee})
			if err != nil {
				return err
			}
			return printTaskList(app, list)
		},
	}
	cmd.Flags().StringVar(&assignee, "assignee", "", "developer ref (required)")
	_ = cmd.MarkFlagRequired("assignee")
	return cmd
}

func newTaskMetadataCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Manage task metadata (key/value pairs)",
	}
	cmd.AddCommand(
		taskMetadataSet(app),
		taskMetadataGet(app),
		taskMetadataDelete(app),
		taskMetadataList(app),
	)
	return cmd
}

func taskMetadataSet(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "set <id> <key> <value>",
		Short: "Set a metadata key on a task",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			t, err := app.Store.SetTaskMetadata(id, args[1], args[2], by)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	return cmd
}

func taskMetadataGet(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id> <key>",
		Short: "Get a metadata value from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			value, err := app.Store.GetTaskMetadata(id, args[1])
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(map[string]string{"key": args[1], "value": value})
			}
			return app.Print.PrintTable([]string{"KEY", "VALUE"}, [][]string{{args[1], value}})
		},
	}
}

func taskMetadataDelete(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "delete <id> <key>",
		Short: "Delete a metadata key from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			t, err := app.Store.DeleteTaskMetadata(id, args[1], by)
			if err != nil {
				return err
			}
			return printTask(app, t)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref")
	return cmd
}

func taskMetadataList(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list <id>",
		Short: "List metadata for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseIDArg(args[0])
			if err != nil {
				return err
			}
			t, err := app.Store.GetTask(id)
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(t.Metadata)
			}
			keys := make([]string, 0, len(t.Metadata))
			for k := range t.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			rows := make([][]string, 0, len(keys))
			for _, k := range keys {
				rows = append(rows, []string{k, t.Metadata[k]})
			}
			return app.Print.PrintTable([]string{"KEY", "VALUE"}, rows)
		},
	}
}

func printTask(app *App, t *model.Task) error {
	if app.Print.PreferStructured() {
		return app.Print.PrintStructured(t)
	}
	rows := [][]string{taskRow(t)}
	return app.Print.PrintTable(taskHeaders(), rows)
}

func printTaskList(app *App, list []model.Task) error {
	if app.Print.PreferStructured() {
		return app.Print.PrintStructured(list)
	}
	rows := make([][]string, 0, len(list))
	for i := range list {
		rows = append(rows, taskRow(&list[i]))
	}
	return app.Print.PrintTable(taskHeaders(), rows)
}

func taskHeaders() []string {
	return []string{"ID", "BOARD", "COLUMN", "TITLE", "PRIORITY", "ASSIGNEE", "TAGS", "WATCHERS"}
}

func taskRow(t *model.Task) []string {
	return []string{
		strconv.FormatInt(t.ID, 10),
		strconv.FormatInt(t.BoardID, 10),
		t.ColumnName,
		t.Title,
		string(t.Priority),
		t.AssigneeRef,
		joinCSV(t.Tags),
		joinCSV(t.WatcherRefs),
	}
}
