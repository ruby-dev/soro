// Package cli implements the Soro command-line interface.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/database"
	"github.com/datasoro/soro/generate"
	"github.com/datasoro/soro/migrate"
	"github.com/spf13/cobra"
)

const DevelopmentVersion = "0.0.0-dev"

type Runner interface {
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer, string, ...string) error
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, directory string, environment []string, stdin io.Reader, stdout, stderr io.Writer, name string, arguments ...string) error {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type Settings struct {
	Directory      string
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	Runner         Runner
	Version        string
	CreateDatabase func(context.Context, string) error
	DropDatabase   func(context.Context, string) error
}

type commandState struct {
	directory      string
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
	runner         Runner
	version        string
	createDatabase func(context.Context, string) error
	dropDatabase   func(context.Context, string) error
}

func New(settings Settings) *cobra.Command {
	if settings.Directory == "" {
		settings.Directory = "."
	}
	if settings.Stdin == nil {
		settings.Stdin = os.Stdin
	}
	if settings.Stdout == nil {
		settings.Stdout = os.Stdout
	}
	if settings.Stderr == nil {
		settings.Stderr = os.Stderr
	}
	if settings.Runner == nil {
		settings.Runner = OSRunner{}
	}
	if settings.Version == "" {
		settings.Version = DevelopmentVersion
	}
	if settings.CreateDatabase == nil {
		settings.CreateDatabase = database.CreateDatabase
	}
	if settings.DropDatabase == nil {
		settings.DropDatabase = database.DropDatabase
	}
	state := &commandState{
		directory: settings.Directory, stdin: settings.Stdin, stdout: settings.Stdout, stderr: settings.Stderr,
		runner: settings.Runner, version: settings.Version,
		createDatabase: settings.CreateDatabase, dropDatabase: settings.DropDatabase,
	}
	root := &cobra.Command{
		Use: "soro", Short: "Build production Go APIs with Soro",
		SilenceUsage: true, SilenceErrors: true,
	}
	root.SetIn(state.stdin)
	root.SetOut(state.stdout)
	root.SetErr(state.stderr)
	root.AddCommand(state.newCommand(), state.serverCommand(), state.routesCommand(), state.generateCommand(), state.databaseCommand(), state.jobsCommand(), state.openapiCommand(), state.versionCommand())
	return root
}

func (state *commandState) newCommand() *cobra.Command {
	var module, frameworkVersion, replacement string
	var force bool
	command := &cobra.Command{
		Use: "new NAME", Short: "Create a new Soro application", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			written, err := generate.NewApplication(generate.NewOptions{
				ParentDirectory: state.directory, Name: arguments[0], Module: module,
				SoroVersion: frameworkVersion, SoroReplace: replacement, Force: force,
			})
			if err != nil {
				return err
			}
			target := filepath.Join(state.directory, arguments[0])
			if err := state.runner.Run(command.Context(), target, []string{"GOWORK=off"}, state.stdin, state.stdout, state.stderr, "go", "mod", "tidy"); err != nil {
				return fmt.Errorf("resolve generated application dependencies: %w", err)
			}
			for _, path := range written {
				fmt.Fprintln(state.stdout, "create", filepath.Join(arguments[0], path))
			}
			return nil
		},
	}
	command.Flags().StringVar(&module, "module", "", "Go module path (defaults to NAME)")
	command.Flags().StringVar(&frameworkVersion, "soro-version", moduleVersion(state.version), "Soro module version")
	command.Flags().StringVar(&replacement, "soro-replace", "", "local Soro source replacement for pre-release development")
	command.Flags().BoolVar(&force, "force", false, "allow generation into an existing directory")
	return command
}

func moduleVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || strings.Contains(version, "dev") {
		return "v0.0.0"
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func (state *commandState) serverCommand() *cobra.Command {
	return &cobra.Command{Use: "server", Short: "Run the application HTTP server", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		return state.runGo(command.Context(), state.stdout, "./cmd/server")
	}}
}

func (state *commandState) routesCommand() *cobra.Command {
	return &cobra.Command{Use: "routes", Short: "List application routes", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		return state.runControl(command.Context(), state.stdout, "routes")
	}}
}

func (state *commandState) jobsCommand() *cobra.Command {
	jobs := &cobra.Command{Use: "jobs", Short: "Manage background jobs"}
	jobs.AddCommand(&cobra.Command{Use: "work", Short: "Run application job workers", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		return state.runControl(command.Context(), state.stdout, "jobs")
	}})
	return jobs
}

func (state *commandState) openapiCommand() *cobra.Command {
	var output string
	var force bool
	openapi := &cobra.Command{Use: "openapi", Short: "Manage the OpenAPI document"}
	generateCommand := &cobra.Command{Use: "generate", Short: "Write the application OpenAPI document", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		absolute := filepath.Join(state.directory, output)
		if _, err := os.Stat(absolute); err == nil && !force {
			return fmt.Errorf("refusing to overwrite %s (use --force)", output)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		var document bytes.Buffer
		if err := state.runControl(command.Context(), &document, "openapi"); err != nil {
			return err
		}
		if err := os.WriteFile(absolute, document.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write OpenAPI document: %w", err)
		}
		fmt.Fprintln(state.stdout, "create", output)
		return nil
	}}
	generateCommand.Flags().StringVarP(&output, "output", "o", "openapi.json", "output path")
	generateCommand.Flags().BoolVar(&force, "force", false, "overwrite an existing document")
	openapi.AddCommand(generateCommand)
	return openapi
}

func (state *commandState) versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the Soro version", Args: cobra.NoArgs, Run: func(_ *cobra.Command, _ []string) {
		fmt.Fprintln(state.stdout, "soro", state.version)
	}}
}

func (state *commandState) generateCommand() *cobra.Command {
	var force bool
	root := &cobra.Command{Use: "generate", Aliases: []string{"g"}, Short: "Generate application components"}
	root.PersistentFlags().BoolVar(&force, "force", false, "overwrite generated files")
	component := func(use, short string, minimum int, execute func(*generate.Generator, string, []string) ([]string, error)) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: cobra.MinimumNArgs(minimum), RunE: func(_ *cobra.Command, arguments []string) error {
			generator, err := generate.Open(state.directory, force)
			if err != nil {
				return err
			}
			written, err := execute(generator, arguments[0], arguments[1:])
			if err != nil {
				return err
			}
			for _, path := range written {
				fmt.Fprintln(state.stdout, "create", path)
			}
			return nil
		}}
	}
	root.AddCommand(
		component("model NAME [FIELD...]", "Generate a model and migration", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			return generator.GenerateModel(name, fields)
		}),
		component("resource NAME [FIELD...]", "Generate a complete CRUD resource", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			return generator.GenerateResource(name, fields)
		}),
		component("serializer NAME [FIELD...]", "Generate a serializer", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			return generator.GenerateSerializer(name, fields)
		}),
		component("validator NAME [FIELD...]", "Generate request validators", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			return generator.GenerateValidator(name, fields)
		}),
		component("migration NAME", "Generate an empty SQL migration", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			if len(fields) != 0 {
				return nil, fmt.Errorf("migration accepts exactly one name")
			}
			return generator.GenerateMigration(name)
		}),
		component("job NAME", "Generate a typed River job", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			if len(fields) != 0 {
				return nil, fmt.Errorf("job accepts exactly one name")
			}
			return generator.GenerateJob(name)
		}),
		component("mailer NAME", "Generate a mailer", 1, func(generator *generate.Generator, name string, fields []string) ([]string, error) {
			if len(fields) != 0 {
				return nil, fmt.Errorf("mailer accepts exactly one name")
			}
			return generator.GenerateMailer(name)
		}),
	)
	return root
}

func (state *commandState) databaseCommand() *cobra.Command {
	root := &cobra.Command{Use: "db", Short: "Manage PostgreSQL and application migrations"}
	root.AddCommand(
		&cobra.Command{Use: "create", Short: "Create the configured PostgreSQL database", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			settings, err := state.loadConfig()
			if err != nil {
				return err
			}
			return state.createDatabase(command.Context(), settings.Database.URL)
		}},
		state.dropCommand(),
		&cobra.Command{Use: "migrate", Short: "Apply application and River migrations", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			return state.withApp(command.Context(), func(app *soro.App) error {
				migrations, err := migrate.LoadDir(filepath.Join(state.directory, "db", "migrations"))
				if err != nil {
					return err
				}
				if err := migrate.New(app.DB).Apply(command.Context(), migrations); err != nil {
					return err
				}
				return app.Jobs.Migrate(command.Context())
			})
		}},
		state.rollbackCommand(),
		&cobra.Command{Use: "status", Short: "Show application migration status", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			return state.withApp(command.Context(), func(app *soro.App) error {
				migrations, err := migrate.LoadDir(filepath.Join(state.directory, "db", "migrations"))
				if err != nil {
					return err
				}
				statuses, err := migrate.New(app.DB).Status(command.Context(), migrations)
				if err != nil {
					return err
				}
				for _, status := range statuses {
					stateName := "down"
					if status.Applied {
						stateName = "up"
					}
					fmt.Fprintf(state.stdout, "%-4s %s\n", stateName, status.Name)
				}
				return nil
			})
		}},
		&cobra.Command{Use: "seed", Short: "Run application seeds", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
			return state.runControl(command.Context(), state.stdout, "seed")
		}},
	)
	return root
}

func (state *commandState) dropCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{Use: "drop", Short: "Drop the configured PostgreSQL database", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if !force {
			return fmt.Errorf("db drop is destructive; pass --force to continue")
		}
		settings, err := state.loadConfig()
		if err != nil {
			return err
		}
		return state.dropDatabase(command.Context(), settings.Database.URL)
	}}
	command.Flags().BoolVar(&force, "force", false, "confirm physical database deletion")
	return command
}

func (state *commandState) rollbackCommand() *cobra.Command {
	var count int
	command := &cobra.Command{Use: "rollback", Short: "Roll back application migrations", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		return state.withApp(command.Context(), func(app *soro.App) error {
			migrations, err := migrate.LoadDir(filepath.Join(state.directory, "db", "migrations"))
			if err != nil {
				return err
			}
			return migrate.New(app.DB).Rollback(command.Context(), migrations, count)
		})
	}}
	command.Flags().IntVarP(&count, "count", "n", 1, "number of migrations to roll back")
	return command
}

func (state *commandState) loadConfig() (*config.Config, error) {
	return config.Load(config.WithDirectory(filepath.Join(state.directory, "config")))
}

func (state *commandState) withApp(ctx context.Context, execute func(*soro.App) error) error {
	settings, err := state.loadConfig()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(state.stderr, nil))
	app, err := soro.New(ctx, soro.WithConfig(settings), soro.WithLogger(logger))
	if err != nil {
		return err
	}
	return errors.Join(execute(app), app.Close())
}

func (state *commandState) runGo(ctx context.Context, output io.Writer, packagePath string, arguments ...string) error {
	allArguments := append([]string{"run", packagePath}, arguments...)
	if err := state.runner.Run(ctx, state.directory, []string{"GOWORK=off"}, state.stdin, output, state.stderr, "go", allArguments...); err != nil {
		return fmt.Errorf("run application command: %w", err)
	}
	return nil
}

func (state *commandState) runControl(ctx context.Context, output io.Writer, operation string) error {
	return state.runGo(ctx, output, "./cmd/app", operation)
}
