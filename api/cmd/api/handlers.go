package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dev.brleinad/butta-la-pasta/internal/response"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
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

type CookingTimes struct {
	CookingTimeMinutes int  `json:"cooking_time_minutes"`
	AlDenteTimeMinutes *int `json:"al_dente_time_minutes,omitempty"`
}

// stripMarkdownCodeFence removes markdown code fences from the content
func stripMarkdownCodeFence(content string) string {
	content = strings.TrimSpace(content)

	// Remove ```json or ``` at the start
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
	}

	// Remove ``` at the end
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSuffix(content, "```")
	}

	return strings.TrimSpace(content)
}

func (app *application) getCookingTimesFromAI(productName string) (*CookingTimes, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Given this pasta product: "%s", use web search to find the recommended cooking time and al dente cooking time in minutes.
Return ONLY a JSON object with this exact structure:
{
  "cooking_time_minutes": <integer>,
  "al_dente_time_minutes": <integer or null>
}

If al dente time is unknown or not applicable, use null.`, productName)

	resp, err := app.openaiClient.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(prompt),
		},
		Model: "gpt-4o-mini",
		Tools: []responses.ToolUnionParam{
			responses.ToolParamOfWebSearchPreview(responses.WebSearchToolTypeWebSearchPreview),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get response from OpenAI: %w", err)
	}

	content := resp.OutputText()

	// Strip markdown code fences if present
	cleanContent := stripMarkdownCodeFence(content)

	var cookingTimes CookingTimes
	if err := json.Unmarshal([]byte(cleanContent), &cookingTimes); err != nil {
		return nil, fmt.Errorf("failed to parse cooking times from AI response: %w", err)
	}

	return &cookingTimes, nil
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

			// Get cooking times from AI
			cookingTimes, aiErr := app.getCookingTimesFromAI(productName)
			if aiErr != nil {
				app.logger.Error("Failed to get cooking times from AI", "error", aiErr, "product", productName)
				// Fallback to default cooking time
				cookingTime := 1
				pasta, err = app.db.CreatePasta(barcode, productName, cookingTime, nil)
			} else {
				app.logger.Info("Got cooking times from AI", "cooking_time", cookingTimes.CookingTimeMinutes, "al_dente_time", cookingTimes.AlDenteTimeMinutes)
				pasta, err = app.db.CreatePasta(barcode, productName, cookingTimes.CookingTimeMinutes, cookingTimes.AlDenteTimeMinutes)
			}

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
