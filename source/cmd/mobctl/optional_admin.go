//go:build admin

package main

func handleOptionalCommand(cfg ctlConfig, command string, args []string) bool {
	switch command {
	case "qterm-ota":
		if err := runOTACommand(cfg, args, qtermOTATarget()); err != nil {
			fatalf("qterm-ota failed: %s", err)
		}
		return true
	case "barcode-ota":
		if err := runOTACommand(cfg, args, barcodeOTATarget()); err != nil {
			fatalf("barcode-ota failed: %s", err)
		}
		return true
	default:
		return false
	}
}

func optionalUsage() string {
	return " | qterm-ota firmware.bin | barcode-ota firmware.bin"
}

func optionalCommands() string {
	return ", qterm-ota path/to/qterm.bin, barcode-ota path/to/station.bin"
}
