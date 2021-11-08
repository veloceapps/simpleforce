package simpleforce

import (
	"fmt"
	"testing"
)

func TestClient_CreateScratch(t *testing.T) {
	client := requireClient(t, true)
 // 	result, err := client.CreateOrRetrieveScratch("vk20 Scratch", "salesforce-environments@secret.com", "MultiCurrency;StateAndCountryPicklist") // for GP orgs
	result, err := client.CreateOrRetrieveScratch("vk20 Scratch", "salesforce-environments@xxxxxxxx.com", "") // for Cato
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	if !result.Success {
		t.FailNow()
	}
	fmt.Printf("Result: %+v\n", result)
}

func TestClient_DropScratch(t *testing.T) {
	client := requireClient(t, true)

	result, err := client.RemoveScratchIfExists("vk20 Scratch")
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	if !result.Success {
		t.FailNow()
	}
	fmt.Printf("Result: %+v", result)
}
