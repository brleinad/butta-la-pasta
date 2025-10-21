package database

import (
	"database/sql"
	"time"
)

type Pasta struct {
	ID                  int64      `db:"id" json:"id"`
	Barcode             string     `db:"barcode" json:"barcode"`
	Name                string     `db:"name" json:"name"`
	CookingTimeMinutes  int        `db:"cooking_time_minutes" json:"cooking_time_minutes"`
	AlDenteTimeMinutes  *int       `db:"al_dente_time_minutes" json:"al_dente_time_minutes,omitempty"`
	CreatedAt           time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time  `db:"updated_at" json:"updated_at"`
}

func (db *DB) GetPastaByBarcode(barcode string) (*Pasta, error) {
	var pasta Pasta

	query := `
		SELECT id, barcode, name, cooking_time_minutes, al_dente_time_minutes, created_at, updated_at
		FROM pasta
		WHERE barcode = ?
	`

	err := db.Get(&pasta, query, barcode)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	return &pasta, nil
}
