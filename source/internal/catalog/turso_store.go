package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "turso.tech/database/tursogo"
)

type TursoStore struct {
	db *sql.DB
}

func OpenTurso(path string) (*TursoStore, error) {
	db, err := sql.Open("turso", path)
	if err != nil {
		return nil, fmt.Errorf("open turso db: %w", err)
	}

	store := &TursoStore{db: db}
	if err := store.ensureSchema(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *TursoStore) Close() error {
	return s.db.Close()
}

func (s *TursoStore) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS products (
  barcode TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  price TEXT NOT NULL,
  stock TEXT NOT NULL,
  unit_of_measure TEXT NOT NULL,
  unit_of_measure_coef TEXT NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func (s *TursoStore) Lookup(ctx context.Context, barcode string) (Product, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT name, barcode, price, stock, unit_of_measure, unit_of_measure_coef
FROM products
WHERE barcode = ?`, barcode)

	var product Product
	if err := row.Scan(&product.Name, &product.Barcode, &product.Price, &product.Stock, &product.UnitOfMeasure, &product.UnitOfMeasureCoef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Product{Barcode: barcode, Valid: false}, nil
		}
		return Product{}, fmt.Errorf("lookup barcode %s: %w", barcode, err)
	}
	product.Valid = true
	return product, nil
}

func (s *TursoStore) Import(ctx context.Context, products <-chan Product, parseErrors <-chan error) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin import: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`DROP TABLE IF EXISTS products_next`,
		`CREATE TABLE products_next (
  barcode TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  price TEXT NOT NULL,
  stock TEXT NOT NULL,
  unit_of_measure TEXT NOT NULL,
  unit_of_measure_coef TEXT NOT NULL
)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return 0, fmt.Errorf("prepare import table: %w", err)
		}
	}

	insert, err := tx.PrepareContext(ctx, `
INSERT OR REPLACE INTO products_next
(barcode, name, price, stock, unit_of_measure, unit_of_measure_coef)
VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare import insert: %w", err)
	}
	defer insert.Close()

	var imported int64
	for product := range products {
		if product.Barcode == "" {
			continue
		}
		_, err := insert.ExecContext(ctx, product.Barcode, product.Name, product.Price, product.Stock, product.UnitOfMeasure, product.UnitOfMeasureCoef)
		if err != nil {
			return imported, fmt.Errorf("insert barcode %s: %w", product.Barcode, err)
		}
		imported++
	}

	if parseErr := <-parseErrors; parseErr != nil {
		return imported, fmt.Errorf("parse catalog: %w", parseErr)
	}

	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS products_next_barcode_idx ON products_next(barcode)`,
		`DROP TABLE IF EXISTS products_old`,
		`ALTER TABLE products RENAME TO products_old`,
		`ALTER TABLE products_next RENAME TO products`,
		`DROP TABLE products_old`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return imported, fmt.Errorf("swap import table: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return imported, fmt.Errorf("commit import: %w", err)
	}
	return imported, nil
}
