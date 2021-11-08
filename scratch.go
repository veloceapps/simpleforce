package simpleforce

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strings"
)

// ExecuteAnonymousResult is returned by ExecuteAnonymous function
type CreateScratchResult struct {
	Features            string      `json:"features"`
	LoginURL            string      `json:"loginUrl"`
	User                string      `json:"user"`
	Pass                string      `json:"pass"`
	AuthCode            string      `json:"authCode"`
	Success             bool        `json:"success"`
	ExceptionStackTrace interface{} `json:"exceptionStackTrace"`
	ExceptionMessage    interface{} `json:"exceptionMessage"`
}

type RemoveScratchResult struct {
	Success             bool        `json:"success"`
	ExceptionStackTrace interface{} `json:"exceptionStackTrace"`
	ExceptionMessage    interface{} `json:"exceptionMessage"`
}

var (
	lowerCharSet   = "abcdedfghijklmnopqrst"
	upperCharSet   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	specialCharSet = "!@#$%&*"
	numberSet      = "0123456789"
	allCharSet     = lowerCharSet + upperCharSet + specialCharSet + numberSet
)

func generatePassword(passwordLength, minSpecialChar, minNum, minUpperCase int) string {
	var password strings.Builder

	//Set special character
	for i := 0; i < minSpecialChar; i++ {
		random := rand.Intn(len(specialCharSet))
		password.WriteString(string(specialCharSet[random]))
	}

	//Set numeric
	for i := 0; i < minNum; i++ {
		random := rand.Intn(len(numberSet))
		password.WriteString(string(numberSet[random]))
	}

	//Set uppercase
	for i := 0; i < minUpperCase; i++ {
		random := rand.Intn(len(upperCharSet))
		password.WriteString(string(upperCharSet[random]))
	}

	remainingLength := passwordLength - minSpecialChar - minNum - minUpperCase
	for i := 0; i < remainingLength; i++ {
		random := rand.Intn(len(allCharSet))
		password.WriteString(string(allCharSet[random]))
	}
	inRune := []rune(password.String())
	rand.Shuffle(len(inRune), func(i, j int) {
		inRune[i], inRune[j] = inRune[j], inRune[i]
	})
	return string(inRune)
}

// CreateOrRetrieveScratch creates or retrieves scratch details based on OrgName
func (client *Client) CreateOrRetrieveScratch(name string, adminEmail string, features string) (*CreateScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}

	// Query active org by OrgName first!
	q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", name)
	result, err := client.Query(q)
	if err != nil {
		return nil, err
	}
	if len(result.Records) > 0 {
		if len(result.Records) > 1 {
			log.Printf("More then one active org with OrgName: %s", name)
		}
		existingOrg := &result.Records[0]
		return &CreateScratchResult{Success: true,
			AuthCode: existingOrg.StringField("AuthCode"),
			User: existingOrg.StringField("SignupUsername"),
			LoginURL: existingOrg.StringField("LoginUrl"),
			Features: existingOrg.StringField("Features"),
		    }, nil
	}

	apexBodyTemplate:= `
      ScratchOrgInfo newScratch = new ScratchOrgInfo (
        OrgName = '%s',
        Edition = 'Developer',
        AdminEmail = '%s',
        ConnectedAppConsumerKey = '%s',
        ConnectedAppCallbackUrl = '%s',
        DurationDays = 30,
        Features = '%s'
      );
      insert(newScratch);
    `
	apexBody := fmt.Sprintf(apexBodyTemplate, name, adminEmail, DefaultClientID, DefaultRedirectURI, features)

	// Create the endpoint
	formatString := "%s/services/data/v%s/tooling/executeAnonymous/?anonymousBody=%s"
	baseURL := client.instanceURL
	endpoint := fmt.Sprintf(formatString, baseURL, client.apiVersion, url.QueryEscape(apexBody))

	data, err := client.httpRequest("GET", endpoint, nil)
	if err != nil {
		log.Println(logPrefix, "HTTP GET request failed:", endpoint)
		return nil, err
	}

	var createResult ExecuteAnonymousResult
	err = json.Unmarshal(data, &createResult)
	if err != nil {
		return nil, err
	}

	if createResult.CompileProblem != nil {
		return &CreateScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
			errors.New(fmt.Sprintf("%+v", createResult.CompileProblem))
	}

	if createResult.ExceptionMessage != nil {
		return &CreateScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
		errors.New(fmt.Sprintf("%+v", createResult.ExceptionMessage))
	}

	if createResult.Success == false {
		return &CreateScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
			errors.New("Unknown error")
	}

	// Query newly created Org
	q = fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", name)
	result, err = client.Query(q)
	if err != nil {
		return nil, err
	}
	var output CreateScratchResult

	if len(result.Records) > 0 {
		if len(result.Records) > 1 {
			log.Printf("More then one active org with OrgName: %s", name)
		}
		existingOrg := &result.Records[0]
		output = CreateScratchResult{Success: true,
			AuthCode: existingOrg.StringField("AuthCode"),
			User: existingOrg.StringField("SignupUsername"),
			LoginURL: existingOrg.StringField("LoginUrl"),
			Features: existingOrg.StringField("Features"),
		}
	} else {
		return &CreateScratchResult{Success: false},
			errors.New("Org Not Found after just created")
	}

	// Set Scratch Password to be Random strong pass
	err = client.LoginWithAuthCode(output.LoginURL, output.AuthCode)
	if err != nil {
		return &CreateScratchResult{Success: false},
			errors.New(fmt.Sprintf("AuthCode auth failed just after org creation: %s", err))
	}

	pass := generatePassword(16, 2,2,2)

	apexBodyTemplate = `
      System.setPassword(userInfo.getUserId(),'%s');
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, pass)

	// Create the endpoint
	formatString = "%s/services/data/v%s/tooling/executeAnonymous/?anonymousBody=%s"
	baseURL = client.instanceURL
	endpoint = fmt.Sprintf(formatString, baseURL, client.apiVersion, url.QueryEscape(apexBody))

	data, err = client.httpRequest("GET", endpoint, nil)
	if err != nil {
		log.Println(logPrefix, "HTTP GET request failed:", endpoint)
		return nil, err
	}

	var setPassResult ExecuteAnonymousResult
	err = json.Unmarshal(data, &setPassResult)
	if err != nil {
		return nil, err
	}

	if setPassResult.CompileProblem != nil {
		return &CreateScratchResult{Success: false, ExceptionMessage: setPassResult.ExceptionMessage, ExceptionStackTrace: setPassResult.ExceptionStackTrace},
			errors.New(fmt.Sprintf("%+v", setPassResult.CompileProblem))
	}

	if setPassResult.ExceptionMessage != nil {
		return &CreateScratchResult{Success: false, ExceptionMessage: setPassResult.ExceptionMessage, ExceptionStackTrace: setPassResult.ExceptionStackTrace},
			errors.New(fmt.Sprintf("%+v", setPassResult.ExceptionMessage))
	}

	if setPassResult.Success == false {
		return &CreateScratchResult{Success: false, ExceptionMessage: setPassResult.ExceptionMessage, ExceptionStackTrace: setPassResult.ExceptionStackTrace},
			errors.New("Unknown error")
	}

	output.Pass = pass;

	return &output, nil

}

// CreateOrRetrieveScratch creates or retrieves scratch details based on OrgName
func (client *Client) RemoveScratchIfExists(name string) (*RemoveScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}

	// Query active org by OrgName first!
	q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 10", name)
	result, err := client.Query(q)
	if err != nil {
		return nil, err
	}
	if len(result.Records) == 0 {
		return &RemoveScratchResult{Success: true}, nil
	}

	apexBodyTemplate:= `
      ScratchOrgInfo[] orgs = [SELECT Id FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 10];
      delete orgs;
    `
	apexBody := fmt.Sprintf(apexBodyTemplate, name)

	// Create the endpoint
	formatString := "%s/services/data/v%s/tooling/executeAnonymous/?anonymousBody=%s"
	baseURL := client.instanceURL
	endpoint := fmt.Sprintf(formatString, baseURL, client.apiVersion, url.QueryEscape(apexBody))

	data, err := client.httpRequest("GET", endpoint, nil)
	if err != nil {
		log.Println(logPrefix, "HTTP GET request failed:", endpoint)
		return nil, err
	}

	var createResult ExecuteAnonymousResult
	err = json.Unmarshal(data, &createResult)
	if err != nil {
		return nil, err
	}

	if createResult.CompileProblem != nil {
		return &RemoveScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
			errors.New(fmt.Sprintf("%+v", createResult.CompileProblem))
	}

	if createResult.ExceptionMessage != nil {
		return &RemoveScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
			errors.New(fmt.Sprintf("%+v", createResult.ExceptionMessage))
	}

	if createResult.Success == false {
		return &RemoveScratchResult{Success: false, ExceptionMessage: createResult.ExceptionMessage, ExceptionStackTrace: createResult.ExceptionStackTrace},
			errors.New("Unknown error")
	}
	return &RemoveScratchResult{Success: true}, nil
}
