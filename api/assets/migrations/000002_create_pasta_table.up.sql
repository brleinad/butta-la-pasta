CREATE TABLE IF NOT EXISTS pasta (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    barcode TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    cooking_time_minutes INTEGER NOT NULL CHECK(cooking_time_minutes > 0),
    al_dente_time_minutes INTEGER CHECK(al_dente_time_minutes > 0),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pasta_barcode ON pasta(barcode);
