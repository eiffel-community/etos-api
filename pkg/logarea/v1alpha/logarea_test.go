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
	"log"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/eiffel-community/etos-api/test/testconfig"
	"github.com/julienschmidt/httprouter"
	"github.com/maxcnunes/httpfake"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/mock/mockserver"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestGetDownloadURLs(t *testing.T) {
}

func startEtcd(cfg *embed.Config) *embed.Etcd {
	srv, err := embed.StartEtcd(cfg)
	if err != nil {
		log.Fatal(err)
	}
	return srv
}

func waitEtcd(srv *embed.Etcd) {
	select {
	case <-srv.Server.ReadyNotify():
		return
	case <-time.After(10 * time.Second):
		srv.Close()
		log.Fatal("Failed to start ETCD server!")
	}
}

func TestGetFileURLs(t *testing.T) {
	log := logrus.NewEntry(logrus.New()).WithField("identifier", t.Name())
	cfg := testconfig.Get("", "", "", "", "")
	srvCfg := embed.NewConfig()
	srvCfg.Dir = "testdata/default.etcd"
	defer os.RemoveAll(srvCfg.Dir)

	fakehttp := httpfake.New(httpfake.WithTesting(t))
	defer fakehttp.Close()
	fakeHandler := fakehttp.NewHandler()
	fakeHandler.Method = "GET"
	fakeHandler.URL.Path = "/"
	fakeHandler.Response.Header.Add("Content-Type", "application/json")
	fakeHandler.Response.BodyString(`{"logs": [{"url": "/file.log","name": "anothername.log"}],"artifacts": [{"url": "/artifact.bin","name": "anothername.bin"}]}`)

	regex := regexp.MustCompile(REGEX)

	srv := startEtcd(srvCfg)
	defer srv.Close()
	waitEtcd(srv)
	cli, err := clientv3.NewFromURL(srvCfg.ListenClientUrls[0].String())
	if err != nil {
		t.Error(err)
	}
	testrunID := "b96d29d9-708c-4cb9-9c43-028675b4f932"
	suiteID := "d4584589-9528-4d6a-a4d7-0954338dfec1"
	subSuiteID := "a427b32c-84b5-4384-b31e-f271dd031098"
	key := fmt.Sprintf("/testrun/%s/suite/%s/subsuite/%s/suite", testrunID, suiteID, subSuiteID)

	suite := Suite{
		Name: t.Name(),
		LogArea: LogArea{
			Download: []Download{
				{
					Request: Request{
						URL:    fakehttp.ResolveURL(""),
						Method: "GET",
					},
					Filters: Filters{
						BaseURL: fakehttp.ResolveURL(""),
						Logs: Filter{
							URLs: []FilterType{{
								Source:   "response",
								JMESPath: "logs[].url",
							}},
							Filename: []FilterType{{
								Source:   "response",
								JMESPath: "logs[].name",
							}},
						},
						Artifacts: Filter{
							URLs: []FilterType{{
								Source:   "response",
								JMESPath: "artifacts[].url",
							}},
							Filename: []FilterType{{
								Source:   "response",
								JMESPath: "artifacts[].name",
							}},
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(suite)

	_, err = cli.Put(context.Background(), key, string(data))
	if err != nil {
		t.Error(err)
	}

	handler := &LogAreaHandler{log, cfg, cli, regex}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", fmt.Sprintf("/v1alpha/logarea/%s", testrunID), nil)
	ps := httprouter.Params{httprouter.Param{Key: "identifier", Value: testrunID}}
	handler.GetFileURLs(responseRecorder, request, ps)

	assert.Equal(t, 200, responseRecorder.Code)
	var response Response
	json.NewDecoder(responseRecorder.Body).Decode(&response)

	assert.NotEmpty(t, response[t.Name()].Logs)
	assert.Equal(t, fakehttp.ResolveURL("/file.log"), response[t.Name()].Logs[0].URL)
	assert.Equal(t, suite.LogArea.Download[0].Filters.Logs.Filename, response[t.Name()].Logs[0].Name)

	assert.NotEmpty(t, response[t.Name()].Artifacts)
	assert.Equal(t, fakehttp.ResolveURL("/artifact.bin"), response[t.Name()].Artifacts[0].URL)
	assert.Equal(t, suite.LogArea.Download[0].Filters.Artifacts.Filename, response[t.Name()].Artifacts[0].Name)
}

func TestGetFileURLsNoTestrun(t *testing.T) {
	log := logrus.NewEntry(logrus.New()).WithField("identifier", t.Name())
	cfg := testconfig.Get("", "", "", "", "")
	regex := regexp.MustCompile(REGEX)

	srv, err := mockserver.StartMockServers(1)
	if err != nil {
		t.Error(err)
	}
	defer srv.Stop()
	cli, err := clientv3.NewFromURL(srv.Servers[0].Address)
	if err != nil {
		t.Error(err)
	}
	handler := &LogAreaHandler{log, cfg, cli, regex}
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1alpha/logarea/12345", nil)
	handler.GetFileURLs(responseRecorder, request, nil)

	assert.Equal(t, 200, responseRecorder.Code)
	var response Response
	json.NewDecoder(responseRecorder.Body).Decode(&response)
	assert.Empty(t, response)
}
