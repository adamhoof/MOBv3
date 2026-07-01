package main

import (
	"encoding/json"
	"testing"
)

func TestParseQRPayArgs(t *testing.T) {
	amount, ref := parseQRPayArgs("A:270,00 VS:26KH03468")
	if amount != "270,00" || ref != "26KH03468" {
		t.Fatalf("unexpected amount/ref: %q %q", amount, ref)
	}
}

func TestParseCommandStripsLine(t *testing.T) {
	cmd, arg, stripped := parseCommand([]byte("head\nCMD:RECIEPT A:1 VS:x\nbody"))
	if cmd != "RECIEPT" || arg != "A:1 VS:x" || string(stripped) != "head\nbody" {
		t.Fatalf("unexpected parse: cmd=%q arg=%q stripped=%q", cmd, arg, stripped)
	}
}

func TestBuildQTermPaymentPayload(t *testing.T) {
	payload, err := buildQTermPaymentPayload("270.00", "26KH03468", confirmationBankPoll)
	if err != nil {
		t.Fatal(err)
	}
	var decoded qtermPaymentPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Amount != "270.00" || decoded.Ref != "26KH03468" || decoded.Confirmation != confirmationBankPoll {
		t.Fatalf("unexpected payload: %+v", decoded)
	}
}
