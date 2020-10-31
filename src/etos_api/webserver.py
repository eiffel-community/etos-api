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
"""ETOS API webserver."""
import sys
import traceback
import logging
import falcon

from etos_api.middleware import RequireJSON, JSONTranslator
from etos_api.lib.params import Params
from etos_api.lib.validator import SuiteValidator, ValidationError


_LOGGER = logging.getLogger(__name__)
LOGFORMAT = "[%(asctime)s] %(levelname)s:%(message)s"
logging.basicConfig(
    level=logging.INFO, stream=sys.stdout, format=LOGFORMAT, datefmt="%Y-%m-%d %H:%M:%S"
)


class Webserver:
    """ETOS API."""

    @staticmethod
    def on_post(request, response):
        """Handle POST requests. Generate and execute an ETOS test suite."""
        params = Params(request)
        validator = SuiteValidator(params)
        try:
            validator.validate()
        except (ValidationError, AssertionError) as exception:
            response.status = falcon.HTTP_400
            response.media = {
                "error": "Not a valid suite definition provided",
                "details": traceback.format_exc(),
            }
            return
        try:
            result = params.tester.handle()
        except Exception as exc:  # pylint: disable=broad-except
            traceback.print_exc()
            response.status = falcon.HTTP_400
            response.media = {
                "error": str(exc),
                "details": traceback.format_exc(),
            }
            return
        finally:
            params.tester.etos.publisher.stop()
        response.status = falcon.HTTP_200
        response.media = result

    @staticmethod
    def on_head(_, response):
        """Handle HEAD requests. Used by ETOS client to check connectivity."""
        response.status = falcon.HTTP_200
        response.media = {"Connection": "SUCCESSFUL"}


FALCON_APP = falcon.API(middleware=[RequireJSON(), JSONTranslator()])
DEV = Webserver()

FALCON_APP.add_route("/", DEV)
