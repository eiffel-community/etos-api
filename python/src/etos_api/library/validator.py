# Copyright 2020-2023 Axis Communications AB.
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
"""ETOS API suite validator module."""
import logging
import asyncio
import time
from typing import List, Union
from uuid import UUID

import requests

# Pylint refrains from linting C extensions due to arbitrary code execution.
from pydantic import BaseModel  # pylint:disable=no-name-in-module
from pydantic import ValidationError, conlist, constr, field_validator
from pydantic.fields import PrivateAttr
from opentelemetry import trace

from etos_api.library.docker import Docker

# pylint:disable=too-few-public-methods


class TestRunnerValidationCache:
    """Lazy test runner validation via in-memory cache."""

    # Cache for lazy testrunner validation. Keys: container names, values: timestamp.
    # Only passed validations are cached.
    TESTRUNNER_VALIDATION_CACHE = {}
    TESTRUNNER_VALIDATION_WINDOW = 1800  # seconds

    lock = asyncio.Lock()

    @classmethod
    async def get_timestamp(cls, test_runner: str) -> Union[float, None]:
        """Get latest passed validation timestamp for the given testrunner.

        :param test_runner: test runner container name
        :type test_runner: str
        :return: validation timestamp or none if not found
        :rtype: float or NoneType
        """
        async with cls.lock:
            if test_runner in cls.TESTRUNNER_VALIDATION_CACHE:
                return cls.TESTRUNNER_VALIDATION_CACHE[test_runner]
        return None

    @classmethod
    async def set_timestamp(cls, test_runner: str, timestamp: float) -> None:
        """Set passed validation timestamp for the given testrunner.

        :param test_runner: test runner container name
        :type test_runner: str
        :param timestamp: test runner container name
        :type timestamp: float
        :return: none
        :rtype: NoneType
        """
        async with cls.lock:
            cls.TESTRUNNER_VALIDATION_CACHE[test_runner] = timestamp

    @classmethod
    async def remove(cls, test_runner: str) -> None:
        """Remove the given test runner from the validation cache.

        :param test_runner: test runner container name
        :type test_runner: str
        :return: none
        :rtype: NoneType
        """
        async with cls.lock:
            if test_runner in cls.TESTRUNNER_VALIDATION_CACHE:
                del cls.TESTRUNNER_VALIDATION_CACHE[test_runner]

    @classmethod
    async def is_test_runner_valid(cls, test_runner: str) -> bool:
        """Determine if the given test runner is valid.

        :param test_runner: test runner container name
        :type test_runner: str
        :return: validation result from cache
        :rtype: bool
        """
        timestamp = await cls.get_timestamp(test_runner)
        if timestamp is None:
            return False
        if (timestamp + cls.TESTRUNNER_VALIDATION_WINDOW) > time.time():
            return True
        await cls.remove(test_runner)
        return False


class Environment(BaseModel):
    """ETOS suite definion 'ENVIRONMENT' constraint."""

    key: str
    value: dict


class Command(BaseModel):
    """ETOS suite definion 'COMMAND' constraint."""

    key: str
    value: constr(min_length=1)


class Checkout(BaseModel):
    """ETOS suite definion 'CHECKOUT' constraint."""

    key: str
    value: conlist(str, min_length=1)


class Parameters(BaseModel):
    """ETOS suite definion 'PARAMETERS' constraint."""

    key: str
    value: dict


class Execute(BaseModel):
    """ETOS suite definion 'EXECUTE' constraint."""

    key: str
    value: List[str]


class TestRunner(BaseModel):
    """ETOS suite definion 'TEST_RUNNER' constraint."""

    key: str
    value: constr(min_length=1)


class TestCase(BaseModel):
    """ETOS suite definion 'testCase' field."""

    id: str
    tracker: str
    url: str


class Constraint(BaseModel):
    """ETOS suite definion 'constraints' field."""

    key: str
    value: Union[str, list, dict]  # pylint:disable=unsubscriptable-object


class Recipe(BaseModel):
    """ETOS suite definion 'recipes' field."""

    constraints: List[Constraint]
    id: UUID
    testCase: TestCase

    __constraint_models = PrivateAttr(
        {
            "ENVIRONMENT": Environment,
            "COMMAND": Command,
            "CHECKOUT": Checkout,
            "PARAMETERS": Parameters,
            "EXECUTE": Execute,
            "TEST_RUNNER": TestRunner,
        }
    )

    @field_validator("constraints")
    def validate_constraints(cls, value):  # Pydantic requires cls. pylint:disable=no-self-argument
        """Validate the constraints fields for each recipe.

        Validation is done manually because error messages from pydantic
        are not clear enough when using a Union check on the models.
        Pydantic does not check the number of unions either, which is something
        that is required for ETOS.

        :raises ValueError: if there are too many or too few constraints.
        :raises TypeError: If an unknown constraint is detected.
        :raises ValidationError: If constraint model does not validate.

        :param value: The current constraint that is being validated.
        :type value: Any
        :return: Same as value, if validated.
        :rtype: Any
        """
        keys = cls.__constraint_models.default.keys()
        count = dict.fromkeys(keys, 0)
        for constraint in value:
            model = cls.__constraint_models.default.get(constraint.key)
            if model is None:
                keys = tuple(keys)
                raise TypeError(f"Unknown key {constraint.key}, valid keys: {keys}")
            try:
                model(**constraint.model_dump())
            except ValidationError as exception:
                raise ValueError(str(exception)) from exception
            count[constraint.key] += 1
        more_than_one = [key for key, number in count.items() if number > 1]
        if more_than_one:
            raise ValueError(f"Too many instances of keys {more_than_one}. Only 1 allowed.")
        missing = [key for key, number in count.items() if number == 0]
        if missing:
            raise ValueError(f"Too few instances of keys {missing}. At least 1 required.")
        return value


class Suite(BaseModel):
    """ETOS base suite definition."""

    name: str
    priority: int
    recipes: List[Recipe]


class SuiteValidator:
    """Validate ETOS suite definitions to make sure they are executable."""

    logger = logging.getLogger(__name__)

    async def _download_suite(self, test_suite_url):
        """Attempt to download suite.

        :param test_suite_url: URL to test suite to download.
        :type test_suite_url: str
        :return: Downloaded test suite as JSON.
        :rtype: list
        """
        try:
            suite = requests.get(test_suite_url, timeout=60)
            suite.raise_for_status()
        except Exception as exception:  # pylint:disable=broad-except
            raise AssertionError(f"Unable to download suite from {test_suite_url}") from exception
        return suite.json()

    async def validate(self, test_suite_url):
        """Validate the ETOS suite definition.

        :param test_suite_url: URL to test suite that is being executed.
        :type test_suite_url: str
        :raises ValidationError: If the suite did not validate.
        """
        span = trace.get_current_span()
        downloaded_suite = await self._download_suite(test_suite_url)
        assert (
            len(downloaded_suite) > 0
        ), "Suite definition validation unsuccessful - Reason: Empty Test suite definition list"
        for suite_json in downloaded_suite:
            test_runners = set()
            suite = Suite(**suite_json)
            assert suite

            for recipe in suite.recipes:
                for constraint in recipe.constraints:
                    if constraint.key == "TEST_RUNNER":
                        test_runners.add(constraint.value)
            docker = Docker()
            for test_runner in test_runners:
                if await TestRunnerValidationCache.is_test_runner_valid(test_runner):
                    self.logger.info("Using cached test runner validation result: %s", test_runner)
                    continue
                for attempt in range(5):
                    if attempt > 0:
                        span.add_event(f"Test runner validation unsuccessful, retry #{attempt}")
                        self.logger.warning(
                            "Test runner %s validation unsuccessful, retry #%d",
                            test_runner,
                            attempt,
                        )
                    result = await docker.digest(test_runner)
                    if result:
                        # only passed validations shall be cached
                        await TestRunnerValidationCache.set_timestamp(test_runner, time.time())
                        break
                    # Total wait time with 5 attempts: 55 seconds
                    sleep_time = (attempt + 1) ** 2
                    await asyncio.sleep(sleep_time)

                assert result is not None, f"Test runner {test_runner} not found"
