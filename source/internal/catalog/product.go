package catalog

type Product struct {
	Name              string `json:"name"`
	Barcode           string `json:"barcode"`
	Price             string `json:"price"`
	Stock             string `json:"stock"`
	UnitOfMeasure     string `json:"unitOfMeasure"`
	UnitOfMeasureCoef string `json:"unitOfMeasureCoef"`
	Valid             bool   `json:"valid"`
}
