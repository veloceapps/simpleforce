package simpleforce

import (
	"fmt"
	"testing"
)

func TestClient_CreateScratch(t *testing.T) {
	client := requireClient(t, true)
 // 	result, err := client.CreateOrRetrieveScratch("vk20 Scratch", "salesforce-environments@secret.com", "MultiCurrency;StateAndCountryPicklist", "+371 12345678", "Latvia") // for GP orgs
	result, err := client.CreateScratch("vk20 Scratch", "cato-full@velocpq.com.0", "salesforce-environments@xxxxxxxx.com", "", "+371 12345678", "Latvia") // for Cato
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	if !result.Success {
		t.FailNow()
	}
	fmt.Printf("Result: %+v\n", result)
}

func TestClient_HasScratch(t *testing.T) {
	client := requireClient(t, true)
	result, err := client.HasScratch("vk20 Scratch")
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	fmt.Printf("Result: %+v\n", result)
}

func TestClient_DropScratch(t *testing.T) {
	client := requireClient(t, true)

	result, err := client.RemoveScratch("vk20 Scratch")
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	if !result.Success {
		t.FailNow()
	}
	fmt.Printf("Result: %+v", result)
}
