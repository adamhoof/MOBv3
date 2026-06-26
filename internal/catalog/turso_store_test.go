package catalog

import (
	"context"
	"errors"
	"testing"
)

func TestTursoStoreImportAndLookup(t *testing.T) {
	store, err := OpenTurso(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	products := make(chan Product, 2)
	products <- Product{Name: "Tea", Barcode: "123", Price: "10", Stock: "5", UnitOfMeasure: "ks", UnitOfMeasureCoef: "1", Valid: true}
	products <- Product{Name: "Coffee", Barcode: "456", Price: "20", Stock: "2", UnitOfMeasure: "ks", UnitOfMeasureCoef: "1", Valid: true}
	close(products)
	parseErrors := make(chan error, 1)
	parseErrors <- nil

	imported, err := store.Import(context.Background(), products, parseErrors)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 2 {
		t.Fatalf("imported=%d", imported)
	}

	product, err := store.Lookup(context.Background(), "456")
	if err != nil {
		t.Fatal(err)
	}
	if !product.Valid || product.Name != "Coffee" {
		t.Fatalf("unexpected product: %+v", product)
	}

	missing, err := store.Lookup(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Valid || missing.Barcode != "missing" {
		t.Fatalf("unexpected missing product: %+v", missing)
	}
}

func TestTursoStoreDoesNotSwapOnParseError(t *testing.T) {
	store, err := OpenTurso(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	initial := make(chan Product, 1)
	initial <- Product{Name: "Old", Barcode: "123", Price: "1", Stock: "1", UnitOfMeasure: "ks", UnitOfMeasureCoef: "1", Valid: true}
	close(initial)
	initialErrs := make(chan error, 1)
	initialErrs <- nil
	if _, err := store.Import(context.Background(), initial, initialErrs); err != nil {
		t.Fatal(err)
	}

	next := make(chan Product, 1)
	next <- Product{Name: "New", Barcode: "123", Price: "2", Stock: "2", UnitOfMeasure: "ks", UnitOfMeasureCoef: "1", Valid: true}
	close(next)
	nextErrs := make(chan error, 1)
	nextErrs <- errors.New("bad csv")
	if _, err := store.Import(context.Background(), next, nextErrs); err == nil {
		t.Fatal("expected parse error")
	}

	product, err := store.Lookup(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if product.Name != "Old" {
		t.Fatalf("parse error swapped table, got %+v", product)
	}
}
