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

// CreateScratch creates scratch with given OrgName
type CreateScratchParams struct {
	Namespace   string
	Name        string
	Username    string
	AdminEmail  string
	Features    string
	Phone       string
	CountryName string
	CountryCode string
	Settings    ScratchSettings
	Description string
}

type ScratchSettings struct {
	EnableAuditFieldsInactiveOwner bool
}

func (client *Client) CreateScratch(params CreateScratchParams) (*CreateScratchResult, error) {
	if !client.isLoggedIn() {
		return nil, ErrAuthentication
	}

	var apexBodyTemplate string
	var apexBody string
	if params.Namespace == "" {
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
          Language = 'en_US',
          Country = '%s'
        );
        insert(newScratch);
        `
		apexBody := fmt.Sprintf(apexBodyTemplate, params.Name, params.Username, params.AdminEmail, DefaultClientID, DefaultRedirectURI, params.Features, params.Description, params.CountryCode)
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
		q := fmt.Sprintf("SELECT FIELDS(ALL) FROM ScratchOrgInfo WHERE OrgName = '%s' AND Status = 'Active' LIMIT 2", params.Name)
		result, err = client.Query(q)
		if err != nil {
			return nil, err
		}
		if len(result.Records) > 1 {
			return nil, fmt.Errorf("More then one active org with OrgName: %s", params.Name)
		}

		if len(result.Records) == 0 {
			log.Printf("Org %s not Found after just created", params.Name)
			select {
			case <-ctxTimeout.Done():
				return nil, fmt.Errorf("Giving up checking %s after creation, not found, waited 6 minutes", params.Name)

			default:
				continue
			}
		}
		break
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
      String phoneNumber = '%s';
      String countryName = '%s';
      String userId = UserInfo.getUserId();
      User user = [SELECT Id, Name,MobilePhone FROM User WHERE Id =: userId LIMIT 1];

      user.Country = countryName;
      user.MobilePhone = phoneNumber;
      user.LanguageLocaleKey = 'en_US';
      update user;
    `
	apexBody = fmt.Sprintf(apexBodyTemplate, params.Phone, params.CountryName)
	_, err = scratchClient.ExecuteAnonymous(apexBody)
	if err != nil {
		return &output, fmt.Errorf("Error setting user details: %s", err)
	}

	return &output, nil
}

type ApplySecuritySettingsParams struct {
	EnableAuditFieldsInactiveOwner bool
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
	ScratchSecuritySettingsMeta := fmt.Sprintf(
		ScratchSecuritySettingsMetaTpl,
		strconv.FormatBool(params.EnableAuditFieldsInactiveOwner),
	)

	// Add some files to the archive.
	var files = []struct {
		Name, Body string
	}{
		{"package.xml", ScratchPackageXML},
		{"settings/Quote.settings", ScratchQuoteSettingsMeta},
		{"settings/Security.settings", ScratchSecuritySettingsMeta},
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

const ScratchQuoteSettingsMeta = `<?xml version="1.0" encoding="UTF-8"?>
<QuoteSettings xmlns="http://soap.sforce.com/2006/04/metadata">
    <enableQuote>true</enableQuote>
</QuoteSettings>`

const ScratchSecuritySettingsMetaTpl = `<?xml version="1.0" encoding="UTF-8"?>
<SecuritySettings xmlns="http://soap.sforce.com/2006/04/metadata">
    <canUsersGrantLoginAccess>true</canUsersGrantLoginAccess>
    <enableAdminLoginAsAnyUser>false</enableAdminLoginAsAnyUser>
    <enableAuditFieldsInactiveOwner>%s</enableAuditFieldsInactiveOwner>
    <enableAuraSecureEvalPref>true</enableAuraSecureEvalPref>
    <enableRequireHttpsConnection>true</enableRequireHttpsConnection>
    <networkAccess>
        <ipRanges>
            <description>allips254</description>
            <end>255.255.255.255</end>
            <start>254.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips253</description>
            <end>254.255.255.255</end>
            <start>253.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips252</description>
            <end>253.255.255.255</end>
            <start>252.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips251</description>
            <end>252.255.255.255</end>
            <start>251.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips250</description>
            <end>251.255.255.255</end>
            <start>250.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips249</description>
            <end>250.255.255.255</end>
            <start>249.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips248</description>
            <end>249.255.255.255</end>
            <start>248.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips247</description>
            <end>248.255.255.255</end>
            <start>247.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips246</description>
            <end>247.255.255.255</end>
            <start>246.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips245</description>
            <end>246.255.255.255</end>
            <start>245.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips244</description>
            <end>245.255.255.255</end>
            <start>244.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips243</description>
            <end>244.255.255.255</end>
            <start>243.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips242</description>
            <end>243.255.255.255</end>
            <start>242.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips241</description>
            <end>242.255.255.255</end>
            <start>241.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips240</description>
            <end>241.255.255.255</end>
            <start>240.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips239</description>
            <end>240.255.255.255</end>
            <start>239.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips238</description>
            <end>239.255.255.255</end>
            <start>238.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips237</description>
            <end>238.255.255.255</end>
            <start>237.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips236</description>
            <end>237.255.255.255</end>
            <start>236.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips235</description>
            <end>236.255.255.255</end>
            <start>235.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips234</description>
            <end>235.255.255.255</end>
            <start>234.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips233</description>
            <end>234.255.255.255</end>
            <start>233.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips232</description>
            <end>233.255.255.255</end>
            <start>232.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips231</description>
            <end>232.255.255.255</end>
            <start>231.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips230</description>
            <end>231.255.255.255</end>
            <start>230.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips229</description>
            <end>230.255.255.255</end>
            <start>229.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips228</description>
            <end>229.255.255.255</end>
            <start>228.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips227</description>
            <end>228.255.255.255</end>
            <start>227.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips226</description>
            <end>227.255.255.255</end>
            <start>226.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips225</description>
            <end>226.255.255.255</end>
            <start>225.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips224</description>
            <end>225.255.255.255</end>
            <start>224.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips223</description>
            <end>224.255.255.255</end>
            <start>223.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips222</description>
            <end>223.255.255.255</end>
            <start>222.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips221</description>
            <end>222.255.255.255</end>
            <start>221.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips220</description>
            <end>221.255.255.255</end>
            <start>220.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips219</description>
            <end>220.255.255.255</end>
            <start>219.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips218</description>
            <end>219.255.255.255</end>
            <start>218.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips217</description>
            <end>218.255.255.255</end>
            <start>217.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips216</description>
            <end>217.255.255.255</end>
            <start>216.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips215</description>
            <end>216.255.255.255</end>
            <start>215.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips214</description>
            <end>215.255.255.255</end>
            <start>214.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips213</description>
            <end>214.255.255.255</end>
            <start>213.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips212</description>
            <end>213.255.255.255</end>
            <start>212.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips211</description>
            <end>212.255.255.255</end>
            <start>211.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips210</description>
            <end>211.255.255.255</end>
            <start>210.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips209</description>
            <end>210.255.255.255</end>
            <start>209.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips208</description>
            <end>209.255.255.255</end>
            <start>208.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips207</description>
            <end>208.255.255.255</end>
            <start>207.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips206</description>
            <end>207.255.255.255</end>
            <start>206.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips205</description>
            <end>206.255.255.255</end>
            <start>205.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips204</description>
            <end>205.255.255.255</end>
            <start>204.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips203</description>
            <end>204.255.255.255</end>
            <start>203.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips202</description>
            <end>203.255.255.255</end>
            <start>202.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips201</description>
            <end>202.255.255.255</end>
            <start>201.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips200</description>
            <end>201.255.255.255</end>
            <start>200.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips199</description>
            <end>200.255.255.255</end>
            <start>199.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips198</description>
            <end>199.255.255.255</end>
            <start>198.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips197</description>
            <end>198.255.255.255</end>
            <start>197.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips196</description>
            <end>197.255.255.255</end>
            <start>196.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips195</description>
            <end>196.255.255.255</end>
            <start>195.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips194</description>
            <end>195.255.255.255</end>
            <start>194.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips193</description>
            <end>194.255.255.255</end>
            <start>193.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips192</description>
            <end>193.255.255.255</end>
            <start>192.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips191</description>
            <end>192.255.255.255</end>
            <start>191.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips190</description>
            <end>191.255.255.255</end>
            <start>190.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips189</description>
            <end>190.255.255.255</end>
            <start>189.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips188</description>
            <end>189.255.255.255</end>
            <start>188.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips187</description>
            <end>188.255.255.255</end>
            <start>187.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips186</description>
            <end>187.255.255.255</end>
            <start>186.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips185</description>
            <end>186.255.255.255</end>
            <start>185.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips184</description>
            <end>185.255.255.255</end>
            <start>184.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips183</description>
            <end>184.255.255.255</end>
            <start>183.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips182</description>
            <end>183.255.255.255</end>
            <start>182.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips181</description>
            <end>182.255.255.255</end>
            <start>181.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips180</description>
            <end>181.255.255.255</end>
            <start>180.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips179</description>
            <end>180.255.255.255</end>
            <start>179.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips178</description>
            <end>179.255.255.255</end>
            <start>178.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips177</description>
            <end>178.255.255.255</end>
            <start>177.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips176</description>
            <end>177.255.255.255</end>
            <start>176.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips175</description>
            <end>176.255.255.255</end>
            <start>175.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips174</description>
            <end>175.255.255.255</end>
            <start>174.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips173</description>
            <end>174.255.255.255</end>
            <start>173.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips172</description>
            <end>173.255.255.255</end>
            <start>172.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips171</description>
            <end>172.255.255.255</end>
            <start>171.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips170</description>
            <end>171.255.255.255</end>
            <start>170.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips169</description>
            <end>170.255.255.255</end>
            <start>169.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips168</description>
            <end>169.255.255.255</end>
            <start>168.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips167</description>
            <end>168.255.255.255</end>
            <start>167.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips166</description>
            <end>167.255.255.255</end>
            <start>166.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips165</description>
            <end>166.255.255.255</end>
            <start>165.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips164</description>
            <end>165.255.255.255</end>
            <start>164.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips163</description>
            <end>164.255.255.255</end>
            <start>163.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips162</description>
            <end>163.255.255.255</end>
            <start>162.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips161</description>
            <end>162.255.255.255</end>
            <start>161.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips160</description>
            <end>161.255.255.255</end>
            <start>160.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips159</description>
            <end>160.255.255.255</end>
            <start>159.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips158</description>
            <end>159.255.255.255</end>
            <start>158.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips157</description>
            <end>158.255.255.255</end>
            <start>157.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips156</description>
            <end>157.255.255.255</end>
            <start>156.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips155</description>
            <end>156.255.255.255</end>
            <start>155.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips154</description>
            <end>155.255.255.255</end>
            <start>154.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips153</description>
            <end>154.255.255.255</end>
            <start>153.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips152</description>
            <end>153.255.255.255</end>
            <start>152.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips151</description>
            <end>152.255.255.255</end>
            <start>151.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips150</description>
            <end>151.255.255.255</end>
            <start>150.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips149</description>
            <end>150.255.255.255</end>
            <start>149.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips148</description>
            <end>149.255.255.255</end>
            <start>148.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips147</description>
            <end>148.255.255.255</end>
            <start>147.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips146</description>
            <end>147.255.255.255</end>
            <start>146.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips145</description>
            <end>146.255.255.255</end>
            <start>145.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips144</description>
            <end>145.255.255.255</end>
            <start>144.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips143</description>
            <end>144.255.255.255</end>
            <start>143.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips142</description>
            <end>143.255.255.255</end>
            <start>142.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips141</description>
            <end>142.255.255.255</end>
            <start>141.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips140</description>
            <end>141.255.255.255</end>
            <start>140.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips139</description>
            <end>140.255.255.255</end>
            <start>139.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips138</description>
            <end>139.255.255.255</end>
            <start>138.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips137</description>
            <end>138.255.255.255</end>
            <start>137.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips136</description>
            <end>137.255.255.255</end>
            <start>136.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips135</description>
            <end>136.255.255.255</end>
            <start>135.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips134</description>
            <end>135.255.255.255</end>
            <start>134.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips133</description>
            <end>134.255.255.255</end>
            <start>133.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips132</description>
            <end>133.255.255.255</end>
            <start>132.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips131</description>
            <end>132.255.255.255</end>
            <start>131.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips130</description>
            <end>131.255.255.255</end>
            <start>130.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips129</description>
            <end>130.255.255.255</end>
            <start>129.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips128</description>
            <end>129.255.255.255</end>
            <start>128.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips127</description>
            <end>128.255.255.255</end>
            <start>127.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips126</description>
            <end>127.255.255.255</end>
            <start>126.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips125</description>
            <end>126.255.255.255</end>
            <start>125.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips124</description>
            <end>125.255.255.255</end>
            <start>124.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips123</description>
            <end>124.255.255.255</end>
            <start>123.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips122</description>
            <end>123.255.255.255</end>
            <start>122.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips121</description>
            <end>122.255.255.255</end>
            <start>121.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips120</description>
            <end>121.255.255.255</end>
            <start>120.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips119</description>
            <end>120.255.255.255</end>
            <start>119.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips118</description>
            <end>119.255.255.255</end>
            <start>118.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips117</description>
            <end>118.255.255.255</end>
            <start>117.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips116</description>
            <end>117.255.255.255</end>
            <start>116.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips115</description>
            <end>116.255.255.255</end>
            <start>115.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips114</description>
            <end>115.255.255.255</end>
            <start>114.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips113</description>
            <end>114.255.255.255</end>
            <start>113.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips112</description>
            <end>113.255.255.255</end>
            <start>112.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips111</description>
            <end>112.255.255.255</end>
            <start>111.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips110</description>
            <end>111.255.255.255</end>
            <start>110.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips109</description>
            <end>110.255.255.255</end>
            <start>109.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips108</description>
            <end>109.255.255.255</end>
            <start>108.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips107</description>
            <end>108.255.255.255</end>
            <start>107.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips106</description>
            <end>107.255.255.255</end>
            <start>106.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips105</description>
            <end>106.255.255.255</end>
            <start>105.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips104</description>
            <end>105.255.255.255</end>
            <start>104.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips103</description>
            <end>104.255.255.255</end>
            <start>103.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips102</description>
            <end>103.255.255.255</end>
            <start>102.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips101</description>
            <end>102.255.255.255</end>
            <start>101.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips100</description>
            <end>101.255.255.255</end>
            <start>100.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips99</description>
            <end>100.255.255.255</end>
            <start>99.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips98</description>
            <end>99.255.255.255</end>
            <start>98.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips97</description>
            <end>98.255.255.255</end>
            <start>97.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips96</description>
            <end>97.255.255.255</end>
            <start>96.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips95</description>
            <end>96.255.255.255</end>
            <start>95.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips94</description>
            <end>95.255.255.255</end>
            <start>94.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips93</description>
            <end>94.255.255.255</end>
            <start>93.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips92</description>
            <end>93.255.255.255</end>
            <start>92.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips91</description>
            <end>92.255.255.255</end>
            <start>91.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips90</description>
            <end>91.255.255.255</end>
            <start>90.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips89</description>
            <end>90.255.255.255</end>
            <start>89.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips88</description>
            <end>89.255.255.255</end>
            <start>88.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips87</description>
            <end>88.255.255.255</end>
            <start>87.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips86</description>
            <end>87.255.255.255</end>
            <start>86.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips85</description>
            <end>86.255.255.255</end>
            <start>85.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips84</description>
            <end>85.255.255.255</end>
            <start>84.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips83</description>
            <end>84.255.255.255</end>
            <start>83.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips82</description>
            <end>83.255.255.255</end>
            <start>82.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips81</description>
            <end>82.255.255.255</end>
            <start>81.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips80</description>
            <end>81.255.255.255</end>
            <start>80.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips79</description>
            <end>80.255.255.255</end>
            <start>79.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips78</description>
            <end>79.255.255.255</end>
            <start>78.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips77</description>
            <end>78.255.255.255</end>
            <start>77.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips76</description>
            <end>77.255.255.255</end>
            <start>76.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips75</description>
            <end>76.255.255.255</end>
            <start>75.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips74</description>
            <end>75.255.255.255</end>
            <start>74.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips73</description>
            <end>74.255.255.255</end>
            <start>73.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips72</description>
            <end>73.255.255.255</end>
            <start>72.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips71</description>
            <end>72.255.255.255</end>
            <start>71.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips70</description>
            <end>71.255.255.255</end>
            <start>70.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips69</description>
            <end>70.255.255.255</end>
            <start>69.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips68</description>
            <end>69.255.255.255</end>
            <start>68.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips67</description>
            <end>68.255.255.255</end>
            <start>67.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips66</description>
            <end>67.255.255.255</end>
            <start>66.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips65</description>
            <end>66.255.255.255</end>
            <start>65.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips64</description>
            <end>65.255.255.255</end>
            <start>64.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips63</description>
            <end>64.255.255.255</end>
            <start>63.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips62</description>
            <end>63.255.255.255</end>
            <start>62.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips61</description>
            <end>62.255.255.255</end>
            <start>61.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips60</description>
            <end>61.255.255.255</end>
            <start>60.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips59</description>
            <end>60.255.255.255</end>
            <start>59.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips58</description>
            <end>59.255.255.255</end>
            <start>58.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips57</description>
            <end>58.255.255.255</end>
            <start>57.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips56</description>
            <end>57.255.255.255</end>
            <start>56.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips55</description>
            <end>56.255.255.255</end>
            <start>55.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips54</description>
            <end>55.255.255.255</end>
            <start>54.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips53</description>
            <end>54.255.255.255</end>
            <start>53.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips52</description>
            <end>53.255.255.255</end>
            <start>52.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips51</description>
            <end>52.255.255.255</end>
            <start>51.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips50</description>
            <end>51.255.255.255</end>
            <start>50.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips49</description>
            <end>50.255.255.255</end>
            <start>49.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips48</description>
            <end>49.255.255.255</end>
            <start>48.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips47</description>
            <end>48.255.255.255</end>
            <start>47.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips46</description>
            <end>47.255.255.255</end>
            <start>46.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips45</description>
            <end>46.255.255.255</end>
            <start>45.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips44</description>
            <end>45.255.255.255</end>
            <start>44.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips43</description>
            <end>44.255.255.255</end>
            <start>43.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips42</description>
            <end>43.255.255.255</end>
            <start>42.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips41</description>
            <end>42.255.255.255</end>
            <start>41.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips40</description>
            <end>41.255.255.255</end>
            <start>40.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips39</description>
            <end>40.255.255.255</end>
            <start>39.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips38</description>
            <end>39.255.255.255</end>
            <start>38.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips37</description>
            <end>38.255.255.255</end>
            <start>37.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips36</description>
            <end>37.255.255.255</end>
            <start>36.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips35</description>
            <end>36.255.255.255</end>
            <start>35.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips34</description>
            <end>35.255.255.255</end>
            <start>34.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips33</description>
            <end>34.255.255.255</end>
            <start>33.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips32</description>
            <end>33.255.255.255</end>
            <start>32.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips31</description>
            <end>32.255.255.255</end>
            <start>31.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips30</description>
            <end>31.255.255.255</end>
            <start>30.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips29</description>
            <end>30.255.255.255</end>
            <start>29.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips28</description>
            <end>29.255.255.255</end>
            <start>28.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips27</description>
            <end>28.255.255.255</end>
            <start>27.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips26</description>
            <end>27.255.255.255</end>
            <start>26.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips25</description>
            <end>26.255.255.255</end>
            <start>25.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips24</description>
            <end>25.255.255.255</end>
            <start>24.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips23</description>
            <end>24.255.255.255</end>
            <start>23.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips22</description>
            <end>23.255.255.255</end>
            <start>22.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips21</description>
            <end>22.255.255.255</end>
            <start>21.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips20</description>
            <end>21.255.255.255</end>
            <start>20.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips19</description>
            <end>20.255.255.255</end>
            <start>19.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips18</description>
            <end>19.255.255.255</end>
            <start>18.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips17</description>
            <end>18.255.255.255</end>
            <start>17.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips16</description>
            <end>17.255.255.255</end>
            <start>16.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips15</description>
            <end>16.255.255.255</end>
            <start>15.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips14</description>
            <end>15.255.255.255</end>
            <start>14.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips13</description>
            <end>14.255.255.255</end>
            <start>13.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips12</description>
            <end>13.255.255.255</end>
            <start>12.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips11</description>
            <end>12.255.255.255</end>
            <start>11.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips10</description>
            <end>11.255.255.255</end>
            <start>10.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips9</description>
            <end>10.255.255.255</end>
            <start>9.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips8</description>
            <end>9.255.255.255</end>
            <start>8.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips7</description>
            <end>8.255.255.255</end>
            <start>7.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips6</description>
            <end>7.255.255.255</end>
            <start>6.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips5</description>
            <end>6.255.255.255</end>
            <start>5.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips4</description>
            <end>5.255.255.255</end>
            <start>4.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips3</description>
            <end>4.255.255.255</end>
            <start>3.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips2</description>
            <end>3.255.255.255</end>
            <start>2.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips1</description>
            <end>2.255.255.255</end>
            <start>1.0.0.0</start>
        </ipRanges>
        <ipRanges>
            <description>allips0</description>
            <end>1.255.255.255</end>
            <start>0.0.0.0</start>
        </ipRanges>
    </networkAccess>
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
        <sessionTimeout>TwelveHours</sessionTimeout>
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
    <version>53.0</version>
</Package>`
