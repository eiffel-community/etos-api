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
package config

import (
	"flag"
	"time"

	"github.com/sirupsen/logrus"
)

type LogAreaConfig interface {
	Config
	UploadPath() string
	Retention() time.Duration
}

// logAreaCfg implements the LogAreaConfig interface
type logAreaCfg struct {
	Config
	uploadPath string
	retention  time.Duration
}

// NewLogAreaConfig creates a log area config interface based on input parameters or environment variables.
func NewLogAreaConfig() LogAreaConfig {
	var conf logAreaCfg
	defaultRetention, err := time.ParseDuration(EnvOrDefault("RETENTION", "24h"))
	if err != nil {
		logrus.Panic(err)
	}

	flag.StringVar(&conf.uploadPath, "uploadpath", EnvOrDefault("UPLOAD_PATH", "/tmp/files"), "Path to upload files to.")
	flag.DurationVar(&conf.retention, "retention", defaultRetention, "Which retention should configured for files")
	base := load()
	flag.Parse()
	conf.Config = base

	return &conf
}

// UploadPath is the local path where all files and directories uploaded should be stored.
func (c logAreaCfg) UploadPath() string {
	return c.uploadPath
}

// Retention returns the file retention time.
func (c logAreaCfg) Retention() time.Duration {
	return c.retention
}
