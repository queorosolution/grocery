// Command migrate applies the embedded SQL migrations. Run it with the database
// OWNER url (never app_rw): CI and make migrate-local do. The url is read from
// -url or MIGRATE_DATABASE_URL and is never printed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"gp/migrations"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	url := fs.String("url", os.Getenv("MIGRATE_DATABASE_URL"), "owner database URL (or MIGRATE_DATABASE_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *url == "" || fs.NArg() != 1 {
		return fmt.Errorf("usage: migrate [-url URL] up|status|version")
	}
	db, err := sql.Open("pgx", *url)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	p, err := migrations.Provider(db)
	if err != nil {
		return err
	}
	switch cmd := fs.Arg(0); cmd {
	case "up":
		results, err := p.Up(ctx)
		for _, r := range results {
			fmt.Printf("applied %s in %s\n", r.Source.Path, r.Duration)
		}
		if err == nil && len(results) == 0 {
			fmt.Println("database is up to date")
		}
		return err
	case "status":
		results, err := p.Status(ctx)
		for _, r := range results {
			fmt.Printf("%-8s %s\n", r.State, r.Source.Path)
		}
		return err
	case "version":
		v, err := p.GetDBVersion(ctx)
		fmt.Println(v)
		return err
	default:
		return fmt.Errorf("unknown command %q (down is not offered: see 0001_init.sql)", cmd)
	}
}
