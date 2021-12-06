// Copyright 2021 Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package logging

import (
	"github.com/eiffel-community/etos-api/internal/config"
	"github.com/sirupsen/logrus"
	"github.com/snowzach/rotatefilehook"
	"go.elastic.co/ecslogrus"
)

// Setup sets up logging to file with a JSON format and to stdout in text format.
func Setup(cfg config.Config) (*logrus.Logger, error) {
	log := logrus.New()

	logLevel, err := logrus.ParseLevel(cfg.LogLevel())
	if err != nil {
		return log, err
	}
	if filePath := cfg.LogFilePath(); filePath != "" {
		// TODO: Make these parameters configurable.
		// NewRotateFileHook cannot return an error which is why it's set to '_'.
		rotateFileHook, _ := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
			Filename:   filePath,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     0, // days
			Level:      logrus.DebugLevel,
			Formatter: &ecslogrus.Formatter{
				DataKey: "labels",
			},
		})
		log.AddHook(rotateFileHook)
	}
	log.SetLevel(logLevel)
	log.SetFormatter(&logrus.TextFormatter{})
	return log, nil
}
