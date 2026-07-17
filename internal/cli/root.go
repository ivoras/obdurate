package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"obdurate/internal/db"
	"obdurate/internal/store"
)

const defaultDBPath = "./db/obdurate.db"

type App struct {
	DBPath string
	JSON   bool
	CSV    bool
	Print  *Printer
	Store  *store.Store
}

func NewRoot() *cobra.Command {
	app := &App{Print: NewPrinter()}

	root := &cobra.Command{
		Use:           "obd",
		Short:         "Obdurate — CLI project management (kanban)",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Print.SetFlags(app.JSON, app.CSV); err != nil {
				return err
			}
			// Skip DB open for pure meta commands.
			switch cmd.Name() {
			case "help", "completion", "version":
				return nil
			}
			// Don't require DB for root alone (shows help)
			if cmd == cmd.Root() {
				return nil
			}
			sqlDB, err := db.Open(app.DBPath)
			if err != nil {
				return err
			}
			// Store close on process exit is fine; attach cleanup.
			cmd.Root().PersistentPostRun = func(cmd *cobra.Command, args []string) {
				_ = sqlDB.Close()
			}
			app.Store = store.New(sqlDB)
			return nil
		},
	}

	root.PersistentFlags().StringVar(&app.DBPath, "db", defaultDBPath, "path to SQLite database")
	root.PersistentFlags().BoolVar(&app.JSON, "json", false, "output JSON")
	root.PersistentFlags().BoolVar(&app.CSV, "csv", false, "output CSV")

	root.AddCommand(
		newDeveloperCmd(app),
		newProjectCmd(app),
		newBoardCmd(app),
		newColumnCmd(app),
		newTaskCmd(app),
		newActivityCmd(app),
		newExportCmd(app),
		newVersionCmd(),
	)

	return root
}

func Execute() {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		code := exitCode(err)
		if errors.Is(err, store.ErrNotFound) {
			code = 2
		} else if errors.Is(err, store.ErrAlreadyExists) || errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrInvalidInput) {
			code = 3
		}
		os.Exit(code)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("obd 0.1.0")
		},
	}
}
