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
package eventrepository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/eiffel-community/eiffelevents-sdk-go"
)

type environmentResponse struct {
	Items []eiffelevents.EnvironmentDefinedV3 `json:"items"`
}

type testSuiteResponse struct {
	Items []eiffelevents.TestSuiteStartedV3 `json:"items"`
}

type activityResponse struct {
	Items []eiffelevents.ActivityTriggeredV4 `json:"items"`
}

// eventRepository returns the event repository URL to use.
func eventRepository() string {
	return os.Getenv("EVENT_REPOSITORY_HOST")
}

// ActivityTriggered returns an activity triggered event from the event repository
func ActivityTriggered(ctx context.Context, id string) (*eiffelevents.ActivityTriggeredV4, error) {
	query := map[string]string{"meta.id": id, "meta.type": "EiffelActivityTriggeredEvent"}
	body, err := getEvents(ctx, query)
	if err != nil {
		return nil, err
	}
	var event activityResponse
	if err = json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	if len(event.Items) == 0 {
		return nil, errors.New("no sub suite found")
	}
	return &event.Items[0], nil
}

// MainSuiteStarted returns a test suite started event from the event repository
func MainSuiteStarted(ctx context.Context, id string) (*eiffelevents.TestSuiteStartedV3, error) {
	activity, err := ActivityTriggered(ctx, id)
	if err != nil {
		return nil, err
	}
	testSuiteID := activity.Links.FindFirst("CONTEXT")

	query := map[string]string{"meta.id": testSuiteID, "meta.type": "EiffelTestSuiteStartedEvent"}
	body, err := getEvents(ctx, query)
	if err != nil {
		return nil, err
	}
	var event testSuiteResponse
	if err = json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	if len(event.Items) == 0 {
		return nil, errors.New("no sub suite found")
	}
	return &event.Items[0], nil
}

// TestSuiteStarted returns a test suite started event from the event repository
func TestSuiteStarted(ctx context.Context, id string, name string) (*eiffelevents.TestSuiteStartedV3, error) {
	query := map[string]string{"links.target": id, "meta.type": "EiffelTestSuiteStartedEvent", "data.name": name}
	body, err := getEvents(ctx, query)
	if err != nil {
		return nil, err
	}
	var event testSuiteResponse
	if err = json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	if len(event.Items) == 0 {
		return nil, errors.New("no sub suite found")
	}
	return &event.Items[0], nil
}

// EnvironmentDefined returns an environment defined event from the event repository
func EnvironmentDefined(ctx context.Context, id string) (*eiffelevents.EnvironmentDefinedV3, error) {
	query := map[string]string{"meta.id": id, "meta.type": "EiffelEnvironmentDefinedEvent"}
	body, err := getEvents(ctx, query)
	if err != nil {
		return nil, err
	}
	var event environmentResponse
	if err = json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	if len(event.Items) == 0 {
		return nil, errors.New("no environment defined found")
	}
	return &event.Items[0], nil
}

// getEvents queries the event repository and returns the response for others to parse
func getEvents(ctx context.Context, query map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", eventRepository(), nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	for key, value := range query {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == 404 {
		return nil, errors.New("event not found in event repository")
	}

	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
