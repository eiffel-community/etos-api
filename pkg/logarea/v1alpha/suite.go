// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package logarea

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/fernet/fernet-go"
	"github.com/jmespath/go-jmespath"
	"github.com/sirupsen/logrus"
)

// Suite is the partial sub suite definition of ETOS.
type Suite struct {
	Name    string  `json:"name"`
	LogArea LogArea `json:"log_area"`
}

type LogArea struct {
	Download Download `json:"download"`
}

type Download struct {
	Request Request `json:"request"`
	Filters Filters `json:"filters"`
}

type Filters struct {
	BaseURL   string `json:"base_url"`
	Logs      Filter `json:"logs"`
	Artifacts Filter `json:"artifacts"`
}

type FilterType struct {
	Source   string `json:"source"`
	JMESPath string `json:"jmespath"`
}

type Filter struct {
	URLs     []FilterType `json:"urls"`
	Filename []FilterType `json:"filename"`
}

// Run the filters stored in the sub suite definition to get URLs from
// a json blob, headers or the suite defintion (as json).
func (f Filter) Run(jsondata []byte, headers interface{}, suite []byte, baseURL string) ([]Downloadable, error) {
	urls := []Downloadable{}
	for _, filter := range f.URLs {
		var source interface{}
		switch strings.ToLower(filter.Source) {
		case "response":
			if err := json.Unmarshal(jsondata, &source); err != nil {
				return urls, err
			}
		case "suite":
			if err := json.Unmarshal(suite, &source); err != nil {
				return urls, err
			}
		case "headers":
			source = headers
		}
		result, err := jmespath.Search(filter.JMESPath, source)
		if err != nil {
			return urls, err
		}
		for _, url := range result.([]interface{}) {
			urls = append(urls, Downloadable{URL: fmt.Sprintf("%s%v", baseURL, url), Name: f.Filename})
		}
	}
	return urls, nil
}

type Request struct {
	Auth   Auth                   `json:"auth,omitempty"`
	URL    string                 `json:"url"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// Do executes a request using the information in the Request part of the
// ETOS sub suite definition.
func (r Request) Do(ctx context.Context, logger *logrus.Entry) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(r.Method), r.URL, nil)
	if err != nil {
		return nil, err
	}
	query := request.URL.Query()
	for key, value := range r.Params {
		query.Add(key, fmt.Sprintf("%v", value))
	}
	if (r.Auth != Auth{}) {
		request.SetBasicAuth(r.Auth.Username, r.Auth.DecryptPassword(logger))
	}

	request.URL.RawQuery = query.Encode()

	return http.DefaultClient.Do(request)
}

type Auth struct {
	Username string  `json:"username"`
	Password Decrypt `json:"password"`
	AuthType string  `json:"type"`
}

// DecryptPassword decrypts the password in the suite definion using
// a decryption key that has been provided as an environment variable.
func (a Auth) DecryptPassword(logger *logrus.Entry) string {
	envKey := os.Getenv("ETOS_ENCRYPTION_KEY")
	if envKey == "" {
		logger.Warning("No encryption key provided")
		return a.Password.Decrypt.Value
	}
	key, err := fernet.DecodeKeys(envKey)
	if err != nil {
		logger.Warningf("Failed to decode password: %s", err)
		return a.Password.Decrypt.Value
	}
	decrypted := fernet.VerifyAndDecrypt([]byte(a.Password.Decrypt.Value), 0, key)
	return string(decrypted)
}

type Decrypt struct {
	Decrypt struct {
		Value string `json:"value"`
	} `json:"$decrypt"`
}
