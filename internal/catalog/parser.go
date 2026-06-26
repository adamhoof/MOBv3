package catalog

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

func ParseCSV(r io.Reader) (<-chan Product, <-chan error) {
	out := make(chan Product)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		reader := csv.NewReader(r)
		reader.Comma = ';'
		reader.LazyQuotes = true
		reader.FieldsPerRecord = 6

		if _, err := reader.Read(); err != nil {
			errs <- fmt.Errorf("read csv header: %w", err)
			return
		}

		recordNumber := 1

		for {
			record, err := reader.Read()
			if errors.Is(err, io.EOF) {
				errs <- nil
				return
			}
			if err != nil {
				errs <- fmt.Errorf("read csv record %d: %w", recordNumber+1, err)
				return
			}
			recordNumber++
			out <- Product{
				Name:              cleanCSVField(record[0]),
				Barcode:           cleanCSVField(record[1]),
				Price:             cleanCSVField(record[2]),
				UnitOfMeasure:     cleanCSVField(record[3]),
				UnitOfMeasureCoef: cleanCSVField(record[4]),
				Stock:             cleanCSVField(record[5]),
				Valid:             true,
			}
		}
	}()

	return out, errs
}

func cleanCSVField(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"`)
}
