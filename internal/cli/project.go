package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"obdurate/internal/model"
	"obdurate/internal/store"
)

func newProjectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects",
		Aliases: []string{"proj"},
	}
	cmd.AddCommand(
		projectCreate(app),
		projectList(app),
		projectGet(app),
		projectUpdate(app),
		projectDelete(app),
	)
	return cmd
}

func projectCreate(app *App) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := app.Store.CreateProject(name, description)
			if err != nil {
				return err
			}
			return printProject(app, p)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (required)")
	cmd.Flags().StringVar(&description, "description", "", "description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func projectList(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListProjects()
			if err != nil {
				return err
			}
			if app.Print.Mode == OutputJSON {
				return app.Print.PrintJSON(list)
			}
			rows := make([][]string, 0, len(list))
			for _, p := range list {
				rows = append(rows, []string{
					strconv.FormatInt(p.ID, 10), p.Name, p.Description,
				})
			}
			return app.Print.PrintTable([]string{"ID", "NAME", "DESCRIPTION"}, rows)
		},
	}
}

func projectGet(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <ref>",
		Short: "Get project by id or name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := app.Store.ResolveProject(args[0])
			if err != nil {
				return err
			}
			return printProject(app, p)
		},
	}
}

func projectUpdate(app *App) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "update <ref>",
		Short: "Update a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := store.ProjectUpdate{}
			if cmd.Flags().Changed("name") {
				u.Name = &name
			}
			if cmd.Flags().Changed("description") {
				u.Description = &description
			}
			p, err := app.Store.UpdateProject(args[0], u)
			if err != nil {
				return err
			}
			return printProject(app, p)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	return cmd
}

func projectDelete(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <ref>",
		Short: "Delete a project and all boards/tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.DeleteProject(args[0]); err != nil {
				return err
			}
			app.Print.PrintOK(fmt.Sprintf("deleted project %s", args[0]))
			if app.Print.Mode == OutputJSON {
				return app.Print.PrintJSON(map[string]string{"status": "deleted", "ref": args[0]})
			}
			return nil
		},
	}
}

func printProject(app *App, p *model.Project) error {
	if app.Print.Mode == OutputJSON {
		return app.Print.PrintJSON(p)
	}
	rows := [][]string{{
		strconv.FormatInt(p.ID, 10), p.Name, p.Description,
		formatTime(p.CreatedAt), formatTime(p.UpdatedAt),
	}}
	return app.Print.PrintTable([]string{"ID", "NAME", "DESCRIPTION", "CREATED", "UPDATED"}, rows)
}
