# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Generic Kubernetes helpers for all submodules."""
import logging
import os

NAMESPACE_FILE = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
LOGGER = logging.getLogger(__name__)


def namespace() -> str:
    """Get current namespace if available."""
    if not os.path.isfile(NAMESPACE_FILE):
        LOGGER.warning("Not running in Kubernetes? Namespace file not found: %s", NAMESPACE_FILE)
        etos_ns = os.getenv("ETOS_NAMESPACE")
        if etos_ns:
            LOGGER.warning("Defauling to environment variable 'ETOS_NAMESPACE': %s", etos_ns)
        else:
            LOGGER.warning("ETOS_NAMESPACE environment variable not set!")
            raise RuntimeError("Failed to determine Kubernetes namespace!")
        return etos_ns
    with open(NAMESPACE_FILE, encoding="utf-8") as namespace_file:
        return namespace_file.read()
