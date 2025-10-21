package main

import (
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /status", app.status)
	mux.HandleFunc("GET /pasta/{barcode}", app.getPastaByBarcode)

	return app.logAccess(app.recoverPanic(mux))
}
