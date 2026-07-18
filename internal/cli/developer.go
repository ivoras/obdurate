package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"obdurate/internal/model"
	"obdurate/internal/store"
)

func newDeveloperCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "developer",
		Short:   "Manage developers",
		Aliases: []string{"dev", "user"},
	}
	cmd.AddCommand(
		devCreate(app),
		devList(app),
		devGet(app),
		devUpdate(app),
		devDelete(app),
		devTasks(app),
	)
	return cmd
}

func devTasks(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "tasks <ref>",
		Short: "List all tasks assigned to a developer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListTasks(store.TaskFilter{AssigneeRef: args[0]})
			if err != nil {
				return err
			}
			return printTaskList(app, list)
		},
	}
}

func devCreate(app *App) *cobra.Command {
	var name, email, username, slackID, role string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a developer",
		RunE: func(cmd *cobra.Command, args []string) error {
			var slack *string
			if slackID != "" {
				slack = &slackID
			}
			d, err := app.Store.CreateDeveloper(name, email, username, slack, model.Role(role))
			if err != nil {
				return err
			}
			return printDeveloper(app, d)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "full name (required)")
	cmd.Flags().StringVar(&email, "email", "", "email (required)")
	cmd.Flags().StringVar(&username, "username", "", "username (required)")
	cmd.Flags().StringVar(&slackID, "slack-id", "", "Slack user id")
	cmd.Flags().StringVar(&role, "role", "developer", "role: admin|lead|developer|viewer")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("username")
	return cmd
}

func devList(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List developers",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := app.Store.ListDevelopers()
			if err != nil {
				return err
			}
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(list)
			}
			rows := make([][]string, 0, len(list))
			for _, d := range list {
				slack := ""
				if d.SlackID != nil {
					slack = *d.SlackID
				}
				rows = append(rows, []string{
					strconv.FormatInt(d.ID, 10), d.Username, d.Name, d.Email, slack, string(d.Role),
				})
			}
			return app.Print.PrintTable([]string{"ID", "USERNAME", "NAME", "EMAIL", "SLACK_ID", "ROLE"}, rows)
		},
	}
}

func devGet(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <ref>",
		Short: "Get developer by id, email, username, or slack-id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := app.Store.ResolveDeveloper(args[0])
			if err != nil {
				return err
			}
			return printDeveloper(app, d)
		},
	}
}

func devUpdate(app *App) *cobra.Command {
	var name, email, username, slackID, role string
	var clearSlack bool
	cmd := &cobra.Command{
		Use:   "update <ref>",
		Short: "Update a developer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u := store.DeveloperUpdate{}
			if cmd.Flags().Changed("name") {
				u.Name = &name
			}
			if cmd.Flags().Changed("email") {
				u.Email = &email
			}
			if cmd.Flags().Changed("username") {
				u.Username = &username
			}
			if cmd.Flags().Changed("slack-id") {
				u.SlackID = &slackID
			}
			if clearSlack {
				empty := ""
				u.SlackID = &empty
			}
			if cmd.Flags().Changed("role") {
				r := model.Role(role)
				u.Role = &r
			}
			d, err := app.Store.UpdateDeveloper(args[0], u)
			if err != nil {
				return err
			}
			return printDeveloper(app, d)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "full name")
	cmd.Flags().StringVar(&email, "email", "", "email")
	cmd.Flags().StringVar(&username, "username", "", "username")
	cmd.Flags().StringVar(&slackID, "slack-id", "", "Slack user id")
	cmd.Flags().BoolVar(&clearSlack, "clear-slack-id", false, "clear Slack id")
	cmd.Flags().StringVar(&role, "role", "", "role: admin|lead|developer|viewer")
	return cmd
}

func devDelete(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <ref>",
		Short: "Delete a developer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.DeleteDeveloper(args[0]); err != nil {
				return err
			}
			app.Print.PrintOK(fmt.Sprintf("deleted developer %s", args[0]))
			if app.Print.PreferStructured() {
				return app.Print.PrintStructured(map[string]string{"status": "deleted", "ref": args[0]})
			}
			return nil
		},
	}
}

func printDeveloper(app *App, d *model.Developer) error {
	if app.Print.PreferStructured() {
		return app.Print.PrintStructured(d)
	}
	slack := ""
	if d.SlackID != nil {
		slack = *d.SlackID
	}
	rows := [][]string{{
		strconv.FormatInt(d.ID, 10), d.Username, d.Name, d.Email, slack, string(d.Role),
		formatTime(d.CreatedAt), formatTime(d.UpdatedAt),
	}}
	return app.Print.PrintTable(
		[]string{"ID", "USERNAME", "NAME", "EMAIL", "SLACK_ID", "ROLE", "CREATED", "UPDATED"},
		rows,
	)
}
