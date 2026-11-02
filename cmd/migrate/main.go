// Command migrate applies database migrations embedded in the binary.
//
//	migrate up            apply all pending migrations
//	migrate down [N]      roll back N migrations (default 1)
//	migrate version       print the current version
//	migrate force V       force the version (after fixing a dirty state)
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jatinjkagrawal/user-management-poc/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if len(args) == 0 {
		args = []string{"up"}
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	// golang-migrate's pgx/v5 driver registers the "pgx5" scheme.
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgx5URL(dbURL))
	if err != nil {
		return err
	}
	defer m.Close()

	switch args[0] {
	case "up":
		err = m.Up()
	case "down":
		n := 1
		if len(args) > 1 {
			if n, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("invalid step count: %w", err)
			}
		}
		err = m.Steps(-n)
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil && !errors.Is(verr, migrate.ErrNilVersion) {
			return verr
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		return nil
	case "force":
		if len(args) < 2 {
			return errors.New("usage: migrate force VERSION")
		}
		v, perr := strconv.Atoi(args[1])
		if perr != nil {
			return perr
		}
		err = m.Force(v)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func toPgx5URL(u string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if len(u) > len(p) && u[:len(p)] == p {
			return "pgx5://" + u[len(p):]
		}
	}
	return u
}
