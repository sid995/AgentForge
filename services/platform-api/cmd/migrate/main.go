package main

import (
	"fmt"
	"os"

	"github.com/sid995/agentforge/services/platform-api/internal/config"
	"github.com/sid995/agentforge/services/platform-api/internal/database/migrations"
)

func main() {
	configuration, err := config.Load(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid migration configuration:", err)
		os.Exit(1)
	}
	if err := migrations.Apply(configuration.Database.URL, "db/migrations"); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
	fmt.Println("migrations applied")
}
