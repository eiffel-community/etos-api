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
	"os"
)

type KeyConfig interface {
	Config
	PrivateKey() ([]byte, error)
}

// keyCfg implements the KeyConfig interface.
type keyCfg struct {
	Config
	privateKeyPath string
}

// NewKeyConcifg creates a key config interface based on input parameters or environment variables.
func NewKeyConfig() KeyConfig {
	var conf keyCfg

	flag.StringVar(&conf.privateKeyPath, "privatekeypath", os.Getenv("PRIVATE_KEY_PATH"), "Path to a private key to use for signing JWTs.")
	base := load()
	flag.Parse()
	conf.Config = base

	return &conf
}

// PrivateKey reads a private key from disk and returns the content.
func (c *keyCfg) PrivateKey() ([]byte, error) {
	if c.privateKeyPath == "" {
		return nil, nil
	}
	return os.ReadFile(c.privateKeyPath)
}
