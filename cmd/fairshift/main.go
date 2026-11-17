// Command fairshift runs the fairshift web app.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/staal0/fairshift/internal/config"
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/store"
	"github.com/staal0/fairshift/internal/web"
)

var version = "dev"

const usage = `fairshift – fair schedules for teams with too many rules.

Usage:
  fairshift serve   [-addr 127.0.0.1:8080]
  fairshift migrate
  fairshift healthcheck                                      (for container health checks)
  fairshift seed    -org org.yaml [-plan plan.yaml] [-force]   (demo/test data; replaces everything)
  fairshift version

Environment:
  FAIRSHIFT_DSN   MySQL DSN (default fairshift:fairshift@tcp(127.0.0.1:3306)/fairshift)
  FAIRSHIFT_ADDR  listen address (default 127.0.0.1:8080)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(args)
	case "migrate":
		err = withStore(func(*store.Store) error { return nil })
	case "seed":
		err = cmdSeed(args)
	case "healthcheck":
		err = cmdHealthcheck()
	case "version":
		fmt.Println("fairshift", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dsn() string {
	if v := os.Getenv("FAIRSHIFT_DSN"); v != "" {
		return v
	}
	return "fairshift:fairshift@tcp(127.0.0.1:3306)/fairshift"
}

// withStore opens the database, runs migrations and calls fn.
func withStore(fn func(*store.Store) error) error {
	st, err := store.Open(dsn())
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer st.Close()
	if err := st.Migrate(context.Background()); err != nil {
		return err
	}
	return fn(st)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", listenAddr(), "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withStore(func(st *store.Store) error {
		srv, err := web.New(st)
		if err != nil {
			return err
		}
		log.Printf("fairshift %s listening on http://%s", version, *addr)
		return http.ListenAndServe(*addr, srv.Handler())
	})
}

func listenAddr() string {
	if v := os.Getenv("FAIRSHIFT_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:8080"
}

// cmdHealthcheck asks the local server for /healthz.
func cmdHealthcheck() error {
	_, port, err := net.SplitHostPort(listenAddr())
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}

func cmdSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	orgPath := fs.String("org", "", "organisation YAML")
	planPath := fs.String("plan", "", "plan YAML (optional)")
	force := fs.Bool("force", false, "replace existing data")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *orgPath == "" {
		return fmt.Errorf("-org is required")
	}
	setup, err := config.LoadOrg(*orgPath)
	if err != nil {
		return err
	}
	var plan *domain.Roster
	var name string
	if *planPath != "" {
		if plan, name, err = config.LoadRoster(*planPath, setup.Org); err != nil {
			return err
		}
	}
	return withStore(func(st *store.Store) error {
		ctx := context.Background()
		empty, err := st.Empty(ctx)
		if err != nil {
			return err
		}
		if !empty && !*force {
			return fmt.Errorf("database already has data; use -force to replace it")
		}
		if err := st.Seed(ctx, setup, plan, name); err != nil {
			return err
		}
		fmt.Printf("seeded %s: %d people, %d rules\n", setup.Org.Name, len(setup.Org.People), len(setup.RuleDefs))
		return nil
	})
}
