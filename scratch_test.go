package simpleforce

import (
	"fmt"
	"testing"
	"time"
)

func TestClient_CreateScratch(t *testing.T) {
	client := requireClient(t, true)
	// 	result, err := client.CreateOrRetrieveScratch("vk20 Scratch", "salesforce-environments@secret.com", "MultiCurrency;StateAndCountryPicklist", "+371 12345678", "Latvia") // for GP orgs
	dateSuffix := time.Now().UTC().Format("0201-150405")
	userName := "cato-full@velocpq.com." + dateSuffix
	result, err := client.CreateScratch(CreateScratchParams{
		Name:        "vk20 Scratch",
		Username:    userName,
		AdminEmail:  userName,
		Features:    "CoreCpq;EnableSetPasswordInApi;Communities;CustomerCommunityPlus;ProductCatalogManagementPCAddOn;OrderSaveLogicEnabled;OrderSaveBehaviorBoth",
		Phone:       "+1 12345678",
		CountryName: "United States",
		CountryCode: "US",
		Settings: ScratchSettings{
			EnableAuditFieldsInactiveOwner: true,
		},
		Description: "DEMO",
		Edition:     "",
		Release:     "",
	}) // for Cato
	if err != nil {
		fmt.Printf("Error: %s", err)
		fmt.Printf("Result: %+v\n", result)
		t.FailNow()
	}
	if !result.Success {
		fmt.Printf("Result: %+v\n", result)
		t.FailNow()
	}
	fmt.Printf("Result: %+v\n", result)
}

func TestClient_CreateNamespaceScratch(t *testing.T) {
	client := requireClient(t, true)
	// 	result, err := client.CreateOrRetrieveScratch("vk20 Scratch", "salesforce-environments@secret.com", "MultiCurrency;StateAndCountryPicklist", "+371 12345678", "Latvia") // for GP orgs
	dateSuffix := time.Now().UTC().Format("0201-150405")
	result, err := client.CreateScratch(CreateScratchParams{
		Namespace:   "VELOCPQ",
		Name:        "vk20 Scratch",
		Username:    "cato-full@velocpq.com." + dateSuffix,
		AdminEmail:  "salesforce-environments@veloce.com",
		Features:    "",
		Phone:       "+371 12345678",
		CountryName: "United States",
		CountryCode: "US",
		Settings: ScratchSettings{
			EnableAuditFieldsInactiveOwner: true,
		},
		Description: "Demo",
		Release:     "Preview",
	}) // for Cato
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	if !result.Success {
		fmt.Printf("Result: %+v\n", result)
		t.FailNow()
	}
	fmt.Printf("Result: %+v\n", result)
}

func TestClient_HasScratch(t *testing.T) {
	client := requireClient(t, true)
	result, _, err := client.HasScratch("vk20 Scratch")
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

func TestScratchesList(t *testing.T) {
	client := requireClient(t, true)
	result, err := client.Scratches()
	if err != nil {
		fmt.Printf("Error: %s", err)
		t.FailNow()
	}
	fmt.Printf("Result: %+v", result)
}
