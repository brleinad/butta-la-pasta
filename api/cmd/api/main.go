package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"

	"dev.brleinad/butta-la-pasta/internal/database"
	"dev.brleinad/butta-la-pasta/internal/env"
	"dev.brleinad/butta-la-pasta/internal/version"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/openai/openai-go"
)

func main() {
	// Load .env file if it exists
	_ = godotenv.Load(".env")

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	err := run(logger)
	if err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

type config struct {
	baseURL      string
	httpPort     int
	openaiAPIKey string
	db           struct {
		dsn         string
		automigrate bool
	}
}

type application struct {
	config       config
	db           *database.DB
	logger       *slog.Logger
	openaiClient openai.Client
	wg           sync.WaitGroup
}

func run(logger *slog.Logger) error {
	var cfg config

	cfg.baseURL = env.GetString("BASE_URL", "http://localhost:7020")
	cfg.httpPort = env.GetInt("HTTP_PORT", 7020)
	cfg.openaiAPIKey = env.GetString("OPENAI_API_KEY", "")
	cfg.db.dsn = env.GetString("DB_DSN", "db.sqlite?_foreign_keys=on")
	cfg.db.automigrate = env.GetBool("DB_AUTOMIGRATE", true)

	showVersion := flag.Bool("version", false, "display version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	db, err := database.New(cfg.db.dsn, cfg.db.automigrate)
	if err != nil {
		return err
	}
	defer db.Close()

	openaiClient := openai.NewClient()

	app := &application{
		config:       cfg,
		db:           db,
		logger:       logger,
		openaiClient: openaiClient,
	}

	return app.serveHTTP()
}
