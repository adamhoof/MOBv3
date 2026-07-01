package catalog

import (
	"strings"
	"testing"
)

func TestParseCSVStreamsProducts(t *testing.T) {
	products, errs := ParseCSV(strings.NewReader("Nazev;EAN;ProdejDPH;MJ2;MJ2Koef;StavZ\nName ; 123 ; \"10.00\" ; ks ; \"1\" ; \"5\"\nOther;456;20;l;0.5;2\n"))

	var got []Product
	for product := range products {
		got = append(got, product)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d products", len(got))
	}
	if got[0].Name != "Name" || got[0].Barcode != "123" || got[0].Stock != "5" || !got[0].Valid {
		t.Fatalf("unexpected first product: %+v", got[0])
	}
}

func TestParseCSVReportsMalformedRecords(t *testing.T) {
	products, errs := ParseCSV(strings.NewReader("Nazev;EAN;ProdejDPH;MJ2;MJ2Koef;StavZ\ntoo;few;fields\n"))
	for range products {
	}
	if err := <-errs; err == nil {
		t.Fatal("expected parse error")
	}
}
