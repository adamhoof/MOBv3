package main

import (
	"testing"

	"github.com/adamhoof/MOBv3/internal/catalog"
)

func TestNormalizeProductResponseStripsDiacritics(t *testing.T) {
	product := normalizeProductResponse(catalog.Product{
		Name:    "Med lesni kvetovy",
		Price:   "Žluťoučký",
		Barcode: "123",
		Valid:   true,
	})

	if product.Name != "Med lesni kvetovy" {
		t.Fatalf("name=%q", product.Name)
	}
	if product.Price != "Zlutoucky" {
		t.Fatalf("price=%q", product.Price)
	}
}

func TestNormalizeProductResponseLeavesInvalidMostlyUntouched(t *testing.T) {
	product := normalizeProductResponse(catalog.Product{Name: "Ž", Valid: false})
	if product.Name != "Ž" {
		t.Fatalf("name=%q", product.Name)
	}
}
