package simpleforce

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// ExecuteAnonymousResult is returned by ExecuteAnonymous function
type CreateScratchResult struct {
	Namespace string `json:"namespace"`
	Features  string `json:"features"`
	LoginURL  string `json:"loginUrl"`
	User      string `json:"user"`
	Pass      string `json:"pass"`
	AuthCode  string `json:"authCode"`
	Success   bool   `json:"success"`
	ExpiresAt string `json:"expiresAt"`

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
func (client *Client) HasScratch(name string) (bool, string, error) {
	if !client.isLoggedIn() {
		return false, "", ErrAuthentication
	}

	// Query active org by OrgName first!
	q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", name)
	result, err := client.Query(q)
	if err != nil {
		return false, "", err
	}
	if len(result.Records) > 0 {
		if len(result.Records) > 1 {
			return false, "", errors.New(fmt.Sprintf("More then one active org with OrgName: %s", name))
		}
		return true, result.Records[0].StringField("ExpirationDate"), nil
	}
	return false, "", nil
}

func (client *Client) Scratches() (scratches []SObject, err error) {
	if !client.isLoggedIn() {
		return scratches, ErrAuthentication
	}

	q := "SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE Status = 'Active' LIMIT 40"
	result, err := client.Query(q)
	if err != nil {
		return scratches, fmt.Errorf("error to query scratches in salesforce devhub: %s", err)
	}
	return result.Records, nil
}

// CreateScratch creates scratch with given OrgName
type CreateScratchParams struct {
	Namespace     string
	Name          string
	Username      string
	AdminEmail    string
	Features      string
	Phone         string
	CountryName   string
	CountryCode   string
	Settings      ScratchSettings
	Description   string
	DurationsDays int
	Edition       string
	Release       string
}

type ScratchSettings struct {
	EnableAuditFieldsInactiveOwner bool
	IPRanges                       []IPRange
}

type IPRange struct {
	Description string
	Start       string
	End         string
}

func buildIPRangesXML(ipRanges []IPRange) string {
	var builder strings.Builder
	for _, ipRange := range ipRanges {
		builder.WriteString(fmt.Sprintf(`        <ipRanges>
            <description>%s</description>
            <end>%s</end>
            <start>%s</start>
        </ipRanges>
`, ipRange.Description, ipRange.End, ipRange.Start))
	}
	return builder.String()
}

func (client *Client) CreateScratch(params CreateScratchParams) (*CreateScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}
	durationDays := 30
	if params.DurationsDays > 0 {
		durationDays = params.DurationsDays
	}

	edition := "Developer"
	if params.Edition != "" {
		edition = params.Edition
	}

	var apexBodyTemplate string
	var apexBody string
	if params.Namespace == "" {
		apexBodyTemplate = `
        ScratchOrgInfo newScratch = new ScratchOrgInfo (
          OrgName = '%s',
          Edition = '%s',
          Username = '%s',
          AdminEmail = '%s',
          ConnectedAppConsumerKey = '%s',
          ConnectedAppCallbackUrl = '%s',
          DurationDays = %d,
          Features = '%s',
          Description = '%s',
          Language = 'en_US',
          Country = '%s',
          Release = '%s'
        );
        insert(newScratch);
        `
		apexBody := fmt.Sprintf(apexBodyTemplate, params.Name, edition, params.Username, params.AdminEmail, DefaultClientID,
			DefaultRedirectURI, durationDays, params.Features, params.Description, params.CountryCode, params.Release)
		_, err := client.ExecuteAnonymous(apexBody)
		if err != nil {
			return nil, err
		}
	} else {
		apexBodyTemplate = `
        ScratchOrgInfo newScratch = new ScratchOrgInfo (
          OrgName = '%s',
          Edition = 'Developer',
          Username = '%s',
          AdminEmail = '%s',
          ConnectedAppConsumerKey = '%s',
          ConnectedAppCallbackUrl = '%s',
          DurationDays = 30,
          Features = '%s',
          Description = '%s',
          Namespace = '%s',
          Language = 'en_US',
          Country = '%s'
        );
        insert(newScratch);
        `
		apexBody := fmt.Sprintf(apexBodyTemplate, params.Name, params.Username, params.AdminEmail, DefaultClientID, DefaultRedirectURI, params.Features, params.Description, params.Namespace, params.CountryCode)
		_, err := client.ExecuteAnonymous(apexBody)
		if err != nil {
			return nil, fmt.Errorf("Error creating scratch org: %s", err)
		}
	}

	var err error
	result := &QueryResult{}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), time.Minute*6)
	defer cancel()
	for {
		time.Sleep(10 * time.Second)

		// Query newly created Org
		q := fmt.Sprintf(
			"SELECT FIELDS(ALL) FROM ScratchOrgInfo "+
				"WHERE OrgName = '%s' AND Username = '%s' AND Status != 'Deleted' LIMIT 2",
			params.Name, params.Username,
		)
		result, err = client.Query(q)
		if err != nil {
			return nil, err
		}
		if len(result.Records) > 1 {
			return nil, fmt.Errorf("More then one  org with OrgName: %s and user: %s", params.Name, params.Username)
		}

		if len(result.Records) == 1 {
			qResult := result.Records[0]
			status := qResult.StringField("Status")
			if status == "Active" {
				log.Printf("Org %s is active, all is ok", params.Name)
				break
			}

			if status == "Error" {
				errorCode := qResult.StringField("ErrorCode")
				return nil, fmt.Errorf("Error status when creating org %s: %s, "+
					"errors explanations: https://developer.salesforce.com/docs/atlas.en-us.sfdx_dev.meta/sfdx_dev/sfdx_dev_scratch_orgs_error_codes.htm",
					params.Name, errorCode,
				)
			}
			log.Printf("Got status %s for org %s when creating with user %s, waiting...", status, params.Name, params.Username)
		}

		if len(result.Records) == 0 {
			log.Printf("Org %s not Found after just created", params.Name)
		}

		select {
		case <-ctxTimeout.Done():

			return nil, fmt.Errorf("Giving up checking %s after creation, not found, waited 6 minutes", params.Name)

		default:
			continue
		}
	}

	var output CreateScratchResult
	existingOrg := &result.Records[0]
	output = CreateScratchResult{Success: true,
		Namespace: existingOrg.StringField("Namespace"),
		AuthCode:  existingOrg.StringField("AuthCode"),
		ExpiresAt: existingOrg.StringField("ExpirationDate"),
		User:      existingOrg.StringField("SignupUsername"),
		LoginURL:  existingOrg.StringField("LoginUrl"),
		Features:  existingOrg.StringField("Features"),
	}

	scratchClient := NewClient(output.LoginURL, DefaultClientID, DefaultAPIVersion)
	err = scratchClient.LoginWithAuthCode(output.LoginURL, output.AuthCode)
	if err != nil {
		return &CreateScratchResult{Success: false},
			errors.New(fmt.Sprintf("AuthCode auth failed just after org creation: %s", err))
	}

	err = scratchClient.ApplySecuritySettings(ApplySecuritySettingsParams{
		EnableAuditFieldsInactiveOwner: params.Settings.EnableAuditFieldsInactiveOwner,
		IPRanges:                       params.Settings.IPRanges,
	})
	if err != nil {
		return &output, fmt.Errorf(`Error applying security settings: %s`, err)
	}

	// Set Scratch Password to be Random strong pass
	pass := generatePassword(16, 2, 2, 2)
	apexBodyTemplate = `
      System.setPassword(userInfo.getUserId(),'%s');
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, pass)
	_, err = scratchClient.ExecuteAnonymous(apexBody)
	if err != nil {
		return &CreateScratchResult{Success: false}, err
	}
	output.Pass = pass

	// Set Phone number in scratch to avoid prompts on UI
	apexBodyTemplate = `
      String countryName = '%s';
      String userId = UserInfo.getUserId();
      User user = [SELECT Id, Name,MobilePhone,DefaultCurrencyIsoCode,CurrencyIsoCode FROM User WHERE Id =: userId LIMIT 1];

      user.Country = countryName;
      user.LanguageLocaleKey = 'en_US';
      user.DefaultCurrencyIsoCode = 'USD';
      user.CurrencyIsoCode = 'USD';
      update user;
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, params.CountryName)
	_, err = scratchClient.ExecuteAnonymous(apexBody)
	if err != nil {
		return &output, fmt.Errorf("Error setting user details: %s", err)
	}

	return &output, nil
}

type ApplySecuritySettingsParams struct {
	EnableAuditFieldsInactiveOwner bool
	IPRanges                       []IPRange
}

func (client *Client) ApplySecuritySettings(params ApplySecuritySettingsParams) error {
	// APPLY Security settings to allow authorizing without 2FA
	/* zip layout:
	package.xml
	settings/Quote.settings
	settings/Security.settings
	*/
	buf := new(bytes.Buffer)
	// Create a new zip archive.
	zipWriter := zip.NewWriter(buf)

	ipRangesXML := buildIPRangesXML(params.IPRanges)
	ScratchSecuritySettingsMeta := fmt.Sprintf(
		ScratchSecuritySettingsMetaTpl,
		strconv.FormatBool(params.EnableAuditFieldsInactiveOwner),
		ipRangesXML,
	)

	// Add some files to the archive.
	var files = []struct {
		Name, Body string
	}{
		{"package.xml", ScratchPackageXML},
		{"settings/Quote.settings", ScratchQuoteSettingsMeta},
		{"settings/Security.settings", ScratchSecuritySettingsMeta},
		{"settings/Currency.settings", ScratchCurrencySettingsMeta},
		{"settings/Deployment.settings", DeploymentSettingsMeta},
	}
	for _, file := range files {
		zipFile, err := zipWriter.Create(file.Name)
		if err != nil {
			return err
		}
		_, err = zipFile.Write([]byte(file.Body))
		if err != nil {
			return err
		}
	}

	// Make sure to check the error on Close.
	err := zipWriter.Close()
	if err != nil {
		return err
	}

	res, err := client.MetaDeploy(buf.Bytes(), "NoTestRun")
	if err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("err: %s, details: %s", res.ErrorMessage, res.Details)
	}
	problems := []string{}
	detailsMap, ok := res.Details.(map[string]interface{})
	if ok {
		compMessages, ok := detailsMap["allComponentMessages"]
		if ok {
			compMessagesList, ok := compMessages.([]interface{})
			if ok {
				for _, comp := range compMessagesList {
					compMap, ok := comp.(map[string]interface{})
					if ok {
						problem, ok := compMap["problem"]
						if ok {
							if problem != nil {
								problems = append(problems, problem.(string))
							}
						}
					}
				}
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("got errors, applying security settings: %s", strings.Join(problems, ", "))
	}
	return nil
}

func (client *Client) RemoveScratch(name string) (*RemoveScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}

	apexBodyTemplate := `
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

const ScratchPackageXML = `<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <types>
        <members>Security</members>
        <name>Settings</name>
    </types>
    <types>
        <members>Quote</members>
        <name>Settings</name>
    </types>
    <types>
        <members>Currency</members>
        <name>Settings</name>
    </types>
    <types>
    	<members>Deployment</members>
     	<name>Settings</name>
    </types>
    <version>53.0</version>
</Package>`

const ScratchQuoteSettingsMeta = `<?xml version="1.0" encoding="UTF-8"?>
<QuoteSettings xmlns="http://soap.sforce.com/2006/04/metadata">
    <enableQuote>true</enableQuote>
</QuoteSettings>`

const ScratchCurrencySettingsMeta = `<?xml version="1.0" encoding="UTF-8"?>
<CurrencySettings xmlns="http://soap.sforce.com/2006/04/metadata">
    <enableMultiCurrency>true</enableMultiCurrency>
</CurrencySettings>`

const DeploymentSettingsMeta = `<?xml version="1.0" encoding="UTF-8"?>
<DeploymentSettings xmlns="http://soap.sforce.com/2006/04/metadata">
   <doesSkipAsyncApexValidation>true</doesSkipAsyncApexValidation>
</DeploymentSettings>`

const ScratchSecuritySettingsMetaTpl = `<?xml version="1.0" encoding="UTF-8"?>
<SecuritySettings xmlns="http://soap.sforce.com/2006/04/metadata">
    <canUsersGrantLoginAccess>true</canUsersGrantLoginAccess>
    <enableAdminLoginAsAnyUser>false</enableAdminLoginAsAnyUser>
    <enableAuditFieldsInactiveOwner>%s</enableAuditFieldsInactiveOwner>
    <enableAuraSecureEvalPref>true</enableAuraSecureEvalPref>
    <enableRequireHttpsConnection>true</enableRequireHttpsConnection>
    <networkAccess>
%s    </networkAccess>
    <passwordPolicies>
        <complexity>AlphaNumeric</complexity>
        <expiration>Never</expiration>
        <historyRestriction>3</historyRestriction>
        <lockoutInterval>FifteenMinutes</lockoutInterval>
        <maxLoginAttempts>TenAttempts</maxLoginAttempts>
        <minimumPasswordLength>8</minimumPasswordLength>
        <minimumPasswordLifetime>false</minimumPasswordLifetime>
        <obscureSecretAnswer>false</obscureSecretAnswer>
        <questionRestriction>DoesNotContainPassword</questionRestriction>
    </passwordPolicies>
    <sessionSettings>
        <allowUserAuthenticationByCertificate>false</allowUserAuthenticationByCertificate>
        <canConfirmEmailChangeInLightningCommunities>true</canConfirmEmailChangeInLightningCommunities>
        <canConfirmIdentityBySmsOnly>true</canConfirmIdentityBySmsOnly>
        <disableTimeoutWarning>false</disableTimeoutWarning>
        <enableBuiltInAuthenticator>false</enableBuiltInAuthenticator>
        <enableCSPOnEmail>true</enableCSPOnEmail>
        <enableCSRFOnGet>true</enableCSRFOnGet>
        <enableCSRFOnPost>true</enableCSRFOnPost>
        <enableCacheAndAutocomplete>true</enableCacheAndAutocomplete>
        <enableClickjackNonsetupSFDC>true</enableClickjackNonsetupSFDC>
        <enableClickjackNonsetupUser>false</enableClickjackNonsetupUser>
        <enableClickjackNonsetupUserHeaderless>false</enableClickjackNonsetupUserHeaderless>
        <enableClickjackSetup>true</enableClickjackSetup>
        <enableContentSniffingProtection>true</enableContentSniffingProtection>
        <enableLightningLogin>true</enableLightningLogin>
        <enableLightningLoginOnlyWithUserPerm>false</enableLightningLoginOnlyWithUserPerm>
        <enableOauthCorsPolicy>false</enableOauthCorsPolicy>
        <enablePostForSessions>false</enablePostForSessions>
        <enableSMSIdentity>true</enableSMSIdentity>
        <enableU2F>false</enableU2F>
        <enableXssProtection>true</enableXssProtection>
        <enforceIpRangesEveryRequest>false</enforceIpRangesEveryRequest>
        <enforceUserDeviceRevoked>false</enforceUserDeviceRevoked>
        <forceLogoutOnSessionTimeout>true</forceLogoutOnSessionTimeout>
        <forceRelogin>true</forceRelogin>
        <hasRetainedLoginHints>false</hasRetainedLoginHints>
        <hasUserSwitching>true</hasUserSwitching>
        <identityConfirmationOnEmailChange>false</identityConfirmationOnEmailChange>
        <identityConfirmationOnTwoFactorRegistrationEnabled>true</identityConfirmationOnTwoFactorRegistrationEnabled>
        <lockSessionsToDomain>true</lockSessionsToDomain>
        <lockSessionsToIp>false</lockSessionsToIp>
        <sessionTimeout>TwentyFourHours</sessionTimeout>
        <lockerServiceCSP>true</lockerServiceCSP>
        <lockerServiceNext>false</lockerServiceNext>
        <lockerServiceNextControl>false</lockerServiceNextControl>
        <redirectionWarning>true</redirectionWarning>
        <referrerPolicy>true</referrerPolicy>
        <requireHttpOnly>false</requireHttpOnly>
        <useLocalStorageForLogoutUrl>false</useLocalStorageForLogoutUrl>
    </sessionSettings>
    <singleSignOnSettings>
        <enableCaseInsensitiveFederationID>false</enableCaseInsensitiveFederationID>
        <enableMultipleSamlConfigs>true</enableMultipleSamlConfigs>
        <enableSamlJitProvisioning>false</enableSamlJitProvisioning>
        <enableSamlLogin>false</enableSamlLogin>
        <isLoginWithSalesforceCredentialsDisabled>false</isLoginWithSalesforceCredentialsDisabled>
    </singleSignOnSettings>
</SecuritySettings>`
