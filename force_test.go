package simpleforce

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
	"strings"
	"testing"
)

var (
	sfUser  = os.ExpandEnv("${SF_USER}")
	sfPass  = os.ExpandEnv("${SF_PASS}")
	sfToken = os.ExpandEnv("${SF_TOKEN}")
	sfURL   = func() string {
		if os.ExpandEnv("${SF_URL}") != "" {
			return os.ExpandEnv("${SF_URL}")
		} else {
			return DefaultURL
		}
	}()
)

func checkCredentialsAndSkip(t *testing.T) {
	if sfUser == "" || sfPass == "" {
		log.Println(logPrefix, "SF_USER, SF_PASS environment variables are not set.")
		t.Skip()
	}
}

func requireClient(t *testing.T, skippable bool) *Client {
	if skippable {
		checkCredentialsAndSkip(t)
	}

	client := NewClient(sfURL, DefaultClientID, DefaultAPIVersion)
	if client == nil {
		t.Fail()
	}
	err := client.LoginPassword(sfUser, sfPass, sfToken)
	if err != nil {
		t.Fatal()
	}
	return client
}

func TestClient_LoginPassword(t *testing.T) {
	checkCredentialsAndSkip(t)

	client := NewClient(sfURL, DefaultClientID, DefaultAPIVersion)
	if client == nil {
		t.Fatal()
	}

	// Use token
	err := client.LoginPassword(sfUser, sfPass, sfToken)
	if err != nil {
		t.Fail()
	} else {
		log.Println(logPrefix, "sessionID:", client.sessionID)
	}

	err = client.LoginPassword("__INVALID_USER__", "__INVALID_PASS__", "__INVALID_TOKEN__")
	if err == nil {
		t.Fail()
	}
}

func TestClient_LoginPasswordNoToken(t *testing.T) {
	checkCredentialsAndSkip(t)

	client := NewClient(sfURL, DefaultClientID, DefaultAPIVersion)
	if client == nil {
		t.Fatal()
	}

	// Trusted IP must be configured AND the request must be initiated from the trusted IP range.
	err := client.LoginPassword(sfUser, sfPass, "")
	if err != nil {
		t.FailNow()
	} else {
		log.Println(logPrefix, "sessionID:", client.sessionID)
	}
}

func TestClient_LoginOAuth(t *testing.T) {

}

func TestClient_Query(t *testing.T) {
	client := requireClient(t, true)

	q := "SELECT Id,LastModifiedById,LastModifiedDate,ParentId,CommentBody FROM CaseComment"
	result, err := client.Query(q)
	if err != nil {
		log.Println(logPrefix, "query failed,", err)
		t.FailNow()
	}

	log.Println(logPrefix, result.TotalSize, result.Done, result.NextRecordsURL)
	if result.TotalSize < 1 {
		log.Println(logPrefix, "no records returned.")
		t.FailNow()
	}
	for _, record := range result.Records {
		if record.Type() != "CaseComment" {
			t.Fail()
		}
	}
}

func TestClient_Query2(t *testing.T) {
	client := requireClient(t, true)

	q := "Select+id,createdbyid,parentid,parent.casenumber,parent.subject,createdby.name,createdby.alias+from+casecomment"
	result, err := client.Query(q)
	if err != nil {
		t.FailNow()
	}
	if len(result.Records) > 0 {
		comment1 := &result.Records[0]
		case1 := comment1.SObjectField("Case", "Parent").Get()
		if comment1.StringField("ParentId") != case1.ID() {
			t.Fail()
		}
	}
}

func TestClient_ApexREST(t *testing.T) {
	client := requireClient(t, true)

	endpoint := "services/apexrest/my-custom-endpoint"
	result, err := client.ApexREST(endpoint, "POST", strings.NewReader(`{"my-property": "my-value"}`))
	if err != nil {
		log.Println(logPrefix, "request failed,", err)
		t.FailNow()
	}

	log.Println(logPrefix, string(result))
}

func TestClient_QueryLike(t *testing.T) {
	client := requireClient(t, true)

	q := "Select Id, createdby.name, subject from case where subject like '%simpleforce%'"
	result, err := client.Query(q)
	if err != nil {
		t.FailNow()
	}
	if len(result.Records) > 0 {
		case0 := &result.Records[0]
		if !strings.Contains(case0.StringField("Subject"), "simpleforce") {
			t.FailNow()
		}
	}
}

func TestMain(m *testing.M) {
	m.Run()
}

func TestJWTLogin(t *testing.T) {
	// Check if required environment variables are set
	consumerKey := os.Getenv("SF_CONSUMER_KEY")
	username := os.Getenv("SF_USERNAME")
	keyPath := os.Getenv("SF_PRIVATE_KEY_PATH")
	loginURL := os.Getenv("SF_LOGIN_URL")

	if consumerKey == "" || username == "" || keyPath == "" {
		log.Println(logPrefix, "SF_CONSUMER_KEY, SF_USERNAME, and SF_PRIVATE_KEY_PATH environment variables must be set for JWT login test")
		t.Skip("Skipping JWT login test - required environment variables not set")
	}

	if loginURL == "" {
		loginURL = "https://test.salesforce.com"
	}

	// Load the private key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to read private key file %s: %v", keyPath, err)
	}

	// Decode PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		t.Fatal("Failed to decode PEM block from private key file")
	}

	// Parse the private key - try PKCS1 first, then PKCS8
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			t.Fatalf("Failed to parse private key as PKCS1 or PKCS8: PKCS1 error=%v, PKCS8 error=%v", err, err2)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			t.Fatal("Private key is not an RSA key")
		}
	}

	// Create client and login
	client := NewClient(loginURL, DefaultClientID, DefaultAPIVersion)
	err = client.LoginJWT(consumerKey, username, privateKey, loginURL)
	if err != nil {
		t.Fatalf("JWT login failed: %v", err)
	}

	log.Println(logPrefix, "JWT login successful")
	log.Println(logPrefix, "Session ID:", client.sessionID)
	log.Println(logPrefix, "Instance URL:", client.instanceURL)
	res, err := client.Query("Select ID from Organization")
	if err != nil {
		t.Fatalf("Query failed")
	}
	log.Printf("query result: %+v", res)
}
