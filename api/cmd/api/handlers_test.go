// +build integration

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"dev.brleinad/butta-la-pasta/internal/database"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go"
)

// setupTestApp creates a test application instance with an in-memory database
func setupTestApp(t *testing.T) (*application, func()) {
	t.Helper()

	// Load .env file for API keys
	_ = godotenv.Load("../../.env") // From cmd/api to api/.env

	// Check for required API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	// Create test logger that discards output
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Use a temporary file for SQLite database (in-memory doesn't work well with migrations)
	dbFile := fmt.Sprintf("/tmp/test-%d.db", os.Getpid())
	db, err := database.New(dbFile+"?_foreign_keys=on", true)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Clean up database file on cleanup
	originalCleanup := func() {
		db.Close()
		os.Remove(dbFile)
	}

	// Create test config
	cfg := config{
		baseURL:      "http://localhost:7020",
		httpPort:     7020,
		openaiAPIKey: os.Getenv("OPENAI_API_KEY"),
	}
	cfg.db.dsn = dbFile + "?_foreign_keys=on"
	cfg.db.automigrate = true

	// Create OpenAI client
	openaiClient := openai.NewClient()

	app := &application{
		config:       cfg,
		db:           db,
		logger:       logger,
		openaiClient: openaiClient,
	}

	return app, originalCleanup
}

// cleanupPasta removes a pasta entry from the database by barcode
func cleanupPasta(t *testing.T, db *database.DB, barcode string) {
	t.Helper()
	_, err := db.Exec("DELETE FROM pasta WHERE barcode = ?", barcode)
	if err != nil {
		t.Logf("Failed to cleanup pasta %s: %v", barcode, err)
	}
}

func TestGetPastaByBarcode_NewPasta(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	testBarcode := "8076802085738" // Barilla Penne Rigate

	// Ensure pasta is not in database
	cleanupPasta(t, app.db, testBarcode)

	// Create test server
	ts := httptest.NewServer(app.routes())
	defer ts.Close()

	// Make request
	resp, err := http.Get(fmt.Sprintf("%s/pasta/%s", ts.URL, testBarcode))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse response
	var pasta struct {
		ID                 int64  `json:"id"`
		Barcode            string `json:"barcode"`
		Name               string `json:"name"`
		CookingTimeMinutes int    `json:"cooking_time_minutes"`
		AlDenteTimeMinutes *int   `json:"al_dente_time_minutes,omitempty"`
		CreatedAt          string `json:"created_at"`
		UpdatedAt          string `json:"updated_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pasta); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response
	if pasta.Barcode != testBarcode {
		t.Errorf("Expected barcode %s, got %s", testBarcode, pasta.Barcode)
	}

	if pasta.Name != "Penne Rigate N°73" {
		t.Errorf("Expected name 'Penne Rigate N°73', got %s", pasta.Name)
	}

	if pasta.CookingTimeMinutes <= 0 {
		t.Errorf("Expected cooking time > 0, got %d", pasta.CookingTimeMinutes)
	}

	if pasta.AlDenteTimeMinutes == nil {
		t.Error("Expected al_dente_time_minutes to be set")
	} else if *pasta.AlDenteTimeMinutes <= 0 {
		t.Errorf("Expected al_dente_time > 0, got %d", *pasta.AlDenteTimeMinutes)
	}

	if pasta.CreatedAt == "" {
		t.Error("Expected created_at to be set")
	}

	if pasta.UpdatedAt == "" {
		t.Error("Expected updated_at to be set")
	}

	// Verify pasta was saved to database
	dbPasta, err := app.db.GetPastaByBarcode(testBarcode)
	if err != nil {
		t.Fatalf("Failed to get pasta from database: %v", err)
	}

	if dbPasta.Name != pasta.Name {
		t.Errorf("Database pasta name doesn't match: expected %s, got %s", pasta.Name, dbPasta.Name)
	}

	t.Logf("Successfully created pasta: %s with cooking time %d minutes", pasta.Name, pasta.CookingTimeMinutes)
}

func TestGetPastaByBarcode_CachedPasta(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	testBarcode := "8076802085738"

	// Pre-insert pasta into database
	expectedCookingTime := 12
	expectedAlDenteTime := 10
	_, err := app.db.CreatePasta(testBarcode, "Penne Rigate N°73", expectedCookingTime, &expectedAlDenteTime)
	if err != nil {
		t.Fatalf("Failed to create test pasta: %v", err)
	}

	// Create test server
	ts := httptest.NewServer(app.routes())
	defer ts.Close()

	// Make request (should hit cache)
	resp, err := http.Get(fmt.Sprintf("%s/pasta/%s", ts.URL, testBarcode))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse response
	var pasta struct {
		Barcode            string `json:"barcode"`
		Name               string `json:"name"`
		CookingTimeMinutes int    `json:"cooking_time_minutes"`
		AlDenteTimeMinutes *int   `json:"al_dente_time_minutes,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pasta); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response matches pre-inserted data
	if pasta.Barcode != testBarcode {
		t.Errorf("Expected barcode %s, got %s", testBarcode, pasta.Barcode)
	}

	if pasta.CookingTimeMinutes != expectedCookingTime {
		t.Errorf("Expected cooking time %d, got %d", expectedCookingTime, pasta.CookingTimeMinutes)
	}

	if pasta.AlDenteTimeMinutes == nil {
		t.Error("Expected al_dente_time_minutes to be set")
	} else if *pasta.AlDenteTimeMinutes != expectedAlDenteTime {
		t.Errorf("Expected al_dente_time %d, got %d", expectedAlDenteTime, *pasta.AlDenteTimeMinutes)
	}

	t.Logf("Successfully retrieved cached pasta: %s", pasta.Name)
}

func TestGetPastaByBarcode_NotFoundAnywhere(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	testBarcode := "9999999999999" // Non-existent barcode

	// Create test server
	ts := httptest.NewServer(app.routes())
	defer ts.Close()

	// Make request
	resp, err := http.Get(fmt.Sprintf("%s/pasta/%s", ts.URL, testBarcode))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	// Parse error response
	var errorResp struct {
		Error string `json:"Error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errorResp.Error == "" {
		t.Error("Expected error message in response")
	}

	t.Logf("Got expected 404 error: %s", errorResp.Error)
}
