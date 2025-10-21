package main

import (
	"database/sql"
	"errors"
	"net/http"

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

func (app *application) getPastaByBarcode(w http.ResponseWriter, r *http.Request) {
	barcode := r.PathValue("barcode")

	pasta, err := app.db.GetPastaByBarcode(barcode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			app.notFound(w, r)
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
