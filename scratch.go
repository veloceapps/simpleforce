package simpleforce

import (
	"errors"
	"fmt"
	"math/rand"
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

// HasScratch creates scratch with given OrgName
func (client *Client) HasScratch(name string) (bool, error) {
	if !client.isLoggedIn() {
		return false, ErrAuthentication
	}

	// Query active org by OrgName first!
	q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", name)
	result, err := client.Query(q)
	if err != nil {
		return false, err
	}
	if len(result.Records) > 0 {
		if len(result.Records) > 1 {
			return false, errors.New(fmt.Sprintf("More then one active org with OrgName: %s", name))
		}
		return true, nil
	}
	return false, nil
}

// CreateScratch creates scratch with given OrgName
func (client *Client) CreateScratch(name string, adminEmail string, features string, phone string, countryName string) (*CreateScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
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
	_, err := client.ExecuteAnonymous(apexBody)
	if err != nil {
		return nil, err
	}

	// Query newly created Org
	q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", name)
	result, err := client.Query(q)
	if err != nil {
		return nil, err
	}
	var output CreateScratchResult
	if len(result.Records) > 1 {
		return nil, errors.New(fmt.Sprintf("More then one active org with OrgName: %s", name))
	}
	if len(result.Records) == 0 {
		return &CreateScratchResult{Success: false},
			errors.New("Org Not Found after just created")
	}

	existingOrg := &result.Records[0]
	output = CreateScratchResult{Success: true,
		AuthCode: existingOrg.StringField("AuthCode"),
		User: existingOrg.StringField("SignupUsername"),
		LoginURL: existingOrg.StringField("LoginUrl"),
		Features: existingOrg.StringField("Features"),
	}

	scratchClient := NewClient(output.LoginURL, DefaultClientID, DefaultAPIVersion)
	err = scratchClient.LoginWithAuthCode(output.LoginURL, output.AuthCode)
	if err != nil {
		return &CreateScratchResult{Success: false},
			errors.New(fmt.Sprintf("AuthCode auth failed just after org creation: %s", err))
	}

	// Set Scratch Password to be Random strong pass
	pass := generatePassword(16, 2,2,2)
	apexBodyTemplate = `
      System.setPassword(userInfo.getUserId(),'%s');
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, pass)
	_, err = scratchClient.ExecuteAnonymous(apexBody)
	if err != nil {
		return nil, err
	}
	output.Pass = pass;

	// Set Phone number in scratch to avoid prompts on UI
	apexBodyTemplate = `
      String phoneNumber = '%s';
      String countryName = '%s';
      String userId = UserInfo.getUserId();
      User user = [SELECT Id, Name,MobilePhone FROM User WHERE Id =: userId LIMIT 1];

      user.Country = countryName;
      user.MobilePhone = phoneNumber;

      update user;
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, phone, countryName)
	_, err = scratchClient.ExecuteAnonymous(apexBody)
	if err != nil {
		return nil, err
	}
	return &output, nil
}

// RemoveScratch creates or retrieves scratch details based on OrgName
func (client *Client) RemoveScratch(name string) (*RemoveScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}

	apexBodyTemplate:= `
      ScratchOrgInfo[] orgs = [SELECT Id FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 10];
      delete orgs;
    `
	apexBody := fmt.Sprintf(apexBodyTemplate, name)

	_, err := client.ExecuteAnonymous(apexBody)
	if err != nil {
		return nil, err
	}

	return &RemoveScratchResult{Success: true}, nil
}
