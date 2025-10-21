package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"dev.brleinad/butta-la-pasta/internal/response"
)

func (app *application) status(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{
		"Status": "OK",
	}

	err := response.JSON(w, http.StatusOK, data)
	if err != nil {
		app.serverError(w, r, err)
	}
}

type openFoodFactsResponse struct {
	Product struct {
		ProductName string `json:"product_name"`
	} `json:"product"`
	Status int `json:"status"`
}

func fetchProductFromOpenFoodFacts(barcode string) (string, error) {
	url := fmt.Sprintf("https://world.openfoodfacts.net/api/v2/product/%s.json", barcode)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Add Basic Auth header
	auth := base64.StdEncoding.EncodeToString([]byte("off:off"))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("open food facts API returned status %d", resp.StatusCode)
	}

	var offResp openFoodFactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&offResp); err != nil {
		return "", err
	}

	// Status 0 means product not found
	if offResp.Status == 0 {
		return "", fmt.Errorf("product not found in Open Food Facts")
	}

	if offResp.Product.ProductName == "" {
		return "", fmt.Errorf("product name not available")
	}

	return offResp.Product.ProductName, nil
}

func (app *application) getPastaByBarcode(w http.ResponseWriter, r *http.Request) {
	barcode := r.PathValue("barcode")

	pasta, err := app.db.GetPastaByBarcode(barcode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Pasta not in DB, try to fetch from Open Food Facts
			productName, offErr := fetchProductFromOpenFoodFacts(barcode)
			if offErr != nil {
				// Not found in Open Food Facts either
				app.notFound(w, r)
				return
			}

			// Log the product name
			app.logger.Info("Found product in Open Food Facts", "barcode", barcode, "name", productName)

			// Create pasta in DB with hardcoded cooking time
			cookingTime := 1
			pasta, err = app.db.CreatePasta(barcode, productName, cookingTime, nil)
			if err != nil {
				app.serverError(w, r, err)
				return
			}

			// Return the newly created pasta
			err = response.JSON(w, http.StatusOK, pasta)
			if err != nil {
				app.serverError(w, r, err)
			}
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, pasta)
	if err != nil {
		app.serverError(w, r, err)
	}
}
