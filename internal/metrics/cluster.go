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
package metrics

import (
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// clusterLabels uses only explicit Cluster ownership, never a service-name convention.
func clusterLabels() prometheus.Labels {
	namespace := strings.TrimSpace(os.Getenv("ETOS_NAMESPACE"))
	if value, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		namespace = strings.TrimSpace(string(value))
	}
	cluster := strings.TrimSpace(os.Getenv("ETOS_CLUSTER"))
	identity := "unassigned"
	if namespace != "" && cluster != "" && !strings.Contains(namespace, "/") && !strings.Contains(cluster, "/") {
		identity = namespace + "/" + cluster
	}
	return prometheus.Labels{"etos_cluster": identity}
}
