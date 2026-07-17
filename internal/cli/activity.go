package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"obdurate/internal/model"
	"obdurate/internal/store"
)

func newActivityCmd(app *App) *cobra.Command {
	var board, project string
	var taskID int64
	var limit int
	cmd := &cobra.Command{
		Use:   "activity",
		Short: "List activity / comment stream",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListActivity(store.ActivityFilter{
				TaskID:     taskID,
				BoardRef:   board,
				ProjectRef: project,
				Limit:      limit,
			})
			if err != nil {
				return err
			}
			return printActivityList(app, list)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "filter by board")
	cmd.Flags().StringVar(&project, "project", "", "filter by project")
	cmd.Flags().Int64Var(&taskID, "task", 0, "filter by task id")
	cmd.Flags().IntVar(&limit, "limit", 50, "max entries")
	return cmd
}

func newExportCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export data (use --json or --csv)",
	}
	cmd.AddCommand(exportTasks(app))
	return cmd
}

func exportTasks(app *App) *cobra.Command {
	var board, project string
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Export tasks for a board or project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if board == "" && project == "" {
				return fmt.Errorf("%w: --board or --project required", store.ErrInvalidInput)
			}
			// Default to JSON when neither format flag set, for scripting.
			if app.Print.Mode == OutputTable {
				app.Print.Mode = OutputJSON
			}
			list, err := app.Store.ListTasks(store.TaskFilter{
				BoardRef:   board,
				ProjectRef: project,
			})
			if err != nil {
				return err
			}
			return printTaskList(app, list)
		},
	}
	cmd.Flags().StringVar(&board, "board", "", "export board tasks")
	cmd.Flags().StringVar(&project, "project", "", "export project tasks")
	return cmd
}

func printActivityOne(app *App, a *model.Activity) error {
	if app.Print.Mode == OutputJSON {
		return app.Print.PrintJSON(a)
	}
	return printActivityList(app, []model.Activity{*a})
}

func printActivityList(app *App, list []model.Activity) error {
	if app.Print.Mode == OutputJSON {
		return app.Print.PrintJSON(list)
	}
	rows := make([][]string, 0, len(list))
	for _, a := range list {
		task := ""
		if a.TaskID != nil {
			task = strconv.FormatInt(*a.TaskID, 10)
		}
		rows = append(rows, []string{
			strconv.FormatInt(a.ID, 10),
			formatTime(a.CreatedAt),
			a.Kind,
			a.ActorRef,
			task,
			a.Message,
		})
	}
	return app.Print.PrintTable(
		[]string{"ID", "CREATED", "KIND", "ACTOR", "TASK", "MESSAGE"},
		rows,
	)
}
