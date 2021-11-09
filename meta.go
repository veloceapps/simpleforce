package simpleforce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

type MetaDeployResult struct {
	Success             bool        `json:"success"`
	ErrorStatusCode  string         `json:"errorStatusCode"`
	ErrorMessage     string         `json:"errorMessage"`
	Details          interface{}   `json:"details"`
}

type MetaDeployResponseResult struct {
	ID               string        `json:"id"`
	Success          bool          `json:"success"`
	Done             bool          `json:"done"`
	Status           string        `json:"status"`
	ErrorStatusCode  string        `json:"errorStatusCode"`
	ErrorMessage     string        `json:"errorMessage"`
	Details          interface{}   `json:"details"`
}

type MetaDeployResponse struct {
	ID             string                   `json:"id"`
	DeployResult   MetaDeployResponseResult `json:"deployResult"`
}


// CreateScratch creates scratch with given OrgName
func (c *Client) MetaDeploy(zip []byte, testLevel string) (*MetaDeployResult, error) {
	if !c.isLoggedIn() {
		return nil, ErrAuthentication
	}

	url := fmt.Sprintf("%s/services/data/v%s/metadata/deployRequest", c.instanceURL, DefaultAPIVersion)
	// Prepare a form that you will submit to that URL.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	var fw io.Writer
	var err error


	// Add descriptor
	sr := strings.NewReader(fmt.Sprintf(`{
    "deployOptions" : {
        "allowMissingFiles" : false,
        "autoUpdatePackage" : false,
        "checkOnly" : false,
        "ignoreWarnings" : false,
        "performRetrieve" : false,
        "purgeOnDelete" : false,
        "rollbackOnError" : false,
        "runTests" : null,
        "singlePackage" : true,
        "testLevel" : "%s"
    }
}`, testLevel))
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`,
			"json"))
	h.Set("Content-Type", "application/json")
	if fw, err = w.CreatePart(h); err != nil {
		return &MetaDeployResult{Success: false}, err
	}

	if _, err := io.Copy(fw, sr); err != nil {
		return &MetaDeployResult{Success: false}, err
	}

	// Add zip
	br := bytes.NewReader(zip)

	h = make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			"file", "deploy.zip"))
	h.Set("Content-Type", "application/zip")
	if fw, err = w.CreatePart(h); err != nil {
		return &MetaDeployResult{Success: false}, err
	}
	if _, err := io.Copy(fw, br); err != nil {
		return &MetaDeployResult{Success: false}, err
	}

	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return &MetaDeployResult{Success: false}, err
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.sessionID))

	// Submit the request
	res, err := c.httpClient.Do(req)
	if err != nil {
		return &MetaDeployResult{Success: false}, err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusCreated {
		resBytes, _ := ioutil.ReadAll(res.Body)
		log.Printf("Error from server: %s", string(resBytes))
		return &MetaDeployResult{Success: false}, fmt.Errorf("bad status: %s", res.Status)
	}
	resBytes, _ := ioutil.ReadAll(res.Body)

	var mr MetaDeployResponse
	err = json.Unmarshal(resBytes, &mr)
	if err != nil {
		return &MetaDeployResult{Success: false}, err
	}

	maxWait := 120;
	for {
		if mr.DeployResult.Done {
			break
		}
		maxWait--;
		if maxWait <= 0 {
			mr.DeployResult.Success = false
			mr.DeployResult.ErrorMessage = "Timeout while waiting for result"
			break;
		}
		url = fmt.Sprintf("%s/services/data/v%s/metadata/deployRequest/%s?includeDetails=true", c.instanceURL, DefaultAPIVersion, mr.ID)
		//req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.sessionID))
		resBytes, err = c.httpRequest("GET", url, nil)
		if err != nil {
			return &MetaDeployResult{Success: false}, err
		}
		err = json.Unmarshal(resBytes, &mr)
		if err != nil {
			return &MetaDeployResult{Success: false}, err
		}
		if mr.DeployResult.Done {
			break
		}
		time.Sleep(1*time.Second)
	}

	if !mr.DeployResult.Success {
		return &MetaDeployResult{Success: mr.DeployResult.Success,
			ErrorMessage: mr.DeployResult.ErrorMessage,
			ErrorStatusCode: mr.DeployResult.ErrorStatusCode,
			Details: mr.DeployResult.Details,
		}, errors.New("Deployment Failed")
	}

	return &MetaDeployResult{Success: mr.DeployResult.Success,
		ErrorMessage: mr.DeployResult.ErrorMessage,
		ErrorStatusCode: mr.DeployResult.ErrorStatusCode,
		Details: mr.DeployResult.Details,
	}, nil
}
