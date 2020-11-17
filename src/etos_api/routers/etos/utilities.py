# Copyright 2020 Axis Communications AB.
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
"""Utilities specific for the ETOS endpoint."""
import time
from etos_api.library.graphql import GraphqlQueryHandler
from etos_api.library.graphql_queries import ARTIFACT_QUERY


async def wait_for_artifact_created(etos_library, artifact_identity, timeout=30):
    """Execute graphql query and wait for an artifact created.

    :param etos_library: ETOS library instande.
    :type etos_library: :obj:`etos_lib.etos.ETOS`
    :param artifact_identity: Identity of the artifact to get.
    :type artifact_identity: str
    :param timeout: Maximum time to wait for a response (seconds).
    :type timeout: int
    :return: ArtifactCreated edges from GraphQL.
    :rtype: list
    """
    timeout = time.time() + timeout
    query_handler = GraphqlQueryHandler(etos_library)
    while time.time() < timeout:
        try:
            artifact = await query_handler.execute(ARTIFACT_QUERY % artifact_identity)
            assert artifact is not None
            assert artifact["artifactCreated"]["edges"]
            return artifact["artifactCreated"]["edges"]
        except (AssertionError, KeyError):
            pass
        time.sleep(2)
    return None
