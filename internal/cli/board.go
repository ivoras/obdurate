package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"obdurate/internal/model"
	"obdurate/internal/store"
)

func newBoardCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "board",
		Short: "Manage kanban boards",
	}
	cmd.AddCommand(
		boardCreate(app),
		boardList(app),
		boardGet(app),
		boardUpdate(app),
		boardDelete(app),
		boardShow(app),
	)
	return cmd
}

func boardCreate(app *App) *cobra.Command {
	var project, name, description, by string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a board (default columns: Todo, Doing, Done)",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := app.Store.CreateBoard(project, name, description, by)
			if err != nil {
				return err
			}
			return printBoard(app, b)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id or name (required)")
	cmd.Flags().StringVar(&name, "name", "", "board name (required)")
	cmd.Flags().StringVar(&description, "description", "", "description")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func boardList(app *App) *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List boards",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListBoards(project)
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(list)
			}
			rows := make([][]string, 0, len(list))
			for _, b := range list {
				rows = append(rows, []string{
					strconv.FormatInt(b.ID, 10),
					strconv.FormatInt(b.ProjectID, 10),
					b.Name,
					b.Description,
				})
			}
			return app.Print.PrintTable([]string{"ID", "PROJECT_ID", "NAME", "DESCRIPTION"}, rows)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "filter by project id or name")
	return cmd
}

func boardGet(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <ref>",
		Short: "Get board by id, name, or project/name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := app.Store.ResolveBoard(args[0])
			if err != nil {
				return err
			}
			return printBoard(app, b)
		},
	}
}

func boardUpdate(app *App) *cobra.Command {
	var name, description, by string
	cmd := &cobra.Command{
		Use:   "update <ref>",
		Short: "Update a board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := store.BoardUpdate{ActorRef: by}
			if cmd.Flags().Changed("name") {
				u.Name = &name
			}
			if cmd.Flags().Changed("description") {
				u.Description = &description
			}
			b, err := app.Store.UpdateBoard(args[0], u)
			if err != nil {
				return err
			}
			return printBoard(app, b)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	return cmd
}

func boardDelete(app *App) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "delete <ref>",
		Short: "Delete a board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.DeleteBoard(args[0], by); err != nil {
				return err
			}
			app.Print.PrintOK(fmt.Sprintf("deleted board %s", args[0]))
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(map[string]string{"status": "deleted", "ref": args[0]})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	return cmd
}

func boardShow(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show kanban board view by status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := app.Store.BoardView(args[0])
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(view)
			}
			if app.Print.Mode == OutputCSV {
				var rows [][]string
				for _, col := range view.Columns {
					if len(col.Tasks) == 0 {
						rows = append(rows, []string{col.Column.Name, "", "", "", "", ""})
						continue
					}
					for _, t := range col.Tasks {
						rows = append(rows, []string{
							col.Column.Name,
							strconv.FormatInt(t.ID, 10),
							t.Title,
							string(t.Priority),
							t.AssigneeRef,
							joinCSV(t.Tags),
						})
					}
				}
				return app.Print.PrintTable(
					[]string{"COLUMN", "TASK_ID", "TITLE", "PRIORITY", "ASSIGNEE", "TAGS"},
					rows,
				)
			}
			// Human kanban-style text
			fmt.Fprintf(app.Print.Out, "Board: %s (#%d)\n", view.Board.Name, view.Board.ID)
			if view.Board.Description != "" {
				fmt.Fprintf(app.Print.Out, "%s\n", view.Board.Description)
			}
			fmt.Fprintln(app.Print.Out, strings.Repeat("=", 60))
			for _, col := range view.Columns {
				fmt.Fprintf(app.Print.Out, "\n## %s (%d)\n", col.Column.Name, len(col.Tasks))
				if len(col.Tasks) == 0 {
					fmt.Fprintln(app.Print.Out, "  (empty)")
					continue
				}
				for _, t := range col.Tasks {
					assignee := t.AssigneeRef
					if assignee == "" {
						assignee = "-"
					}
					tags := joinCSV(t.Tags)
					if tags == "" {
						tags = "-"
					}
					fmt.Fprintf(app.Print.Out, "  [%d] %s  prio=%s  assignee=%s  tags=%s\n",
						t.ID, t.Title, t.Priority, assignee, tags)
				}
			}
			return nil
		},
	}
}

func printBoard(app *App, b *model.Board) error {
	if app.Print.PreferStructured() {
		return app.Print.PrintStructured(b)
	}
	rows := [][]string{{
		strconv.FormatInt(b.ID, 10),
		strconv.FormatInt(b.ProjectID, 10),
		b.Name,
		b.Description,
		formatTime(b.CreatedAt),
		formatTime(b.UpdatedAt),
	}}
	return app.Print.PrintTable([]string{"ID", "PROJECT_ID", "NAME", "DESCRIPTION", "CREATED", "UPDATED"}, rows)
}

func newColumnCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "column",
		Short:   "Manage board columns",
		Aliases: []string{"col"},
	}
	cmd.AddCommand(
		columnAdd(app),
		columnList(app),
		columnRename(app),
		columnReorder(app),
		columnDelete(app),
	)
	return cmd
}

func columnAdd(app *App) *cobra.Command {
	var board, name, by string
	var position int
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a column to a board",
		RunE: func(cmd *cobra.Command, args []string) error {
			var pos *int
			if cmd.Flags().Changed("position") {
				pos = &position
			}
			c, err := app.Store.AddColumn(board, name, pos, by)
			if err != nil {
				return err
			}
			return printColumn(app, c)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	cmd.Flags().StringVar(&name, "name", "", "column name (required)")
	cmd.Flags().IntVar(&position, "position", 0, "position index (default: append)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func columnList(app *App) *cobra.Command {
	var board string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List columns on a board",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := app.Store.ResolveBoard(board)
			if err != nil {
				return err
			}
			cols, err := app.Store.ListColumns(b.ID)
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(cols)
			}
			rows := make([][]string, 0, len(cols))
			for _, c := range cols {
				rows = append(rows, []string{
					strconv.FormatInt(c.ID, 10),
					c.Name,
					strconv.Itoa(c.Position),
				})
			}
			return app.Print.PrintTable([]string{"ID", "NAME", "POSITION"}, rows)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	_ = cmd.MarkFlagRequired("board")
	return cmd
}

func columnRename(app *App) *cobra.Command {
	var board, name, by string
	cmd := &cobra.Command{
		Use:   "rename <column>",
		Short: "Rename a column",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.Store.RenameColumn(board, args[0], name, by)
			if err != nil {
				return err
			}
			return printColumn(app, c)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	cmd.Flags().StringVar(&name, "name", "", "new name (required)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func columnReorder(app *App) *cobra.Command {
	var board, by string
	var position int
	cmd := &cobra.Command{
		Use:   "reorder <column>",
		Short: "Change column position",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.Store.ReorderColumn(board, args[0], position, by)
			if err != nil {
				return err
			}
			return printColumn(app, c)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	cmd.Flags().IntVar(&position, "position", 0, "new position index (required)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("position")
	return cmd
}

func columnDelete(app *App) *cobra.Command {
	var board, by string
	cmd := &cobra.Command{
		Use:   "delete <column>",
		Short: "Delete an empty column",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.DeleteColumn(board, args[0], by); err != nil {
				return err
			}
			app.Print.PrintOK(fmt.Sprintf("deleted column %s", args[0]))
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(map[string]string{"status": "deleted", "column": args[0]})
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "board ref (required)")
	cmd.Flags().StringVar(&by, "by", "", "actor developer ref for activity log")
	_ = cmd.MarkFlagRequired("board")
	return cmd
}

func printColumn(app *App, c *model.Column) error {
	if app.Print.PreferStructured() {
		return app.Print.PrintStructured(c)
	}
	rows := [][]string{{
		strconv.FormatInt(c.ID, 10),
		strconv.FormatInt(c.BoardID, 10),
		c.Name,
		strconv.Itoa(c.Position),
	}}
	return app.Print.PrintTable([]string{"ID", "BOARD_ID", "NAME", "POSITION"}, rows)
}
