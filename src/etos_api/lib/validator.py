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
"""ETOS API suite validator module."""
import logging
from uuid import UUID
from typing import Union, List
from pydantic import BaseModel, validator, ValidationError, constr, conlist
import requests


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
    value: conlist(str, min_items=1)


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

    __constraint_models = {
        "ENVIRONMENT": Environment,
        "COMMAND": Command,
        "CHECKOUT": Checkout,
        "PARAMETERS": Parameters,
        "EXECUTE": Execute,
        "TEST_RUNNER": TestRunner,
    }

    @validator("constraints")
    def validate_constraints(
        cls, value
    ):  # Pydantic requires cls. pylint:disable=no-self-argument
        """Validate the constraints fields for each recipe.

        This is a separate method where the validation is done manually.
        That is because the error messages from pydantic are not clear enough
        when using a Union check on the models.
        Pydantic does not check the number of unions either, which is something
        that is required for ETOS.
        """
        count = dict.fromkeys(cls.__constraint_models.keys(), 0)
        for constraint in value:
            model = cls.__constraint_models.get(constraint.key)
            if model is None:
                raise TypeError(
                    "Unknown key %r, valid keys: %r"
                    % (constraint.key, tuple(cls.__constraint_models.keys()))
                )
            try:
                model(**constraint.dict())
            except ValidationError as exception:
                raise ValueError(str(exception)) from exception
            count[constraint.key] += 1
        more_than_one = [key for key, number in count.items() if number > 1]
        if more_than_one:
            raise ValueError(
                "Too many instances of keys %r. Only 1 allowed." % more_than_one
            )
        missing = [key for key, number in count.items() if number == 0]
        if missing:
            raise ValueError(
                "Too few instances of keys %r. At least 1 required." % missing
            )
        return value


class Suite(BaseModel):
    """ETOS base suite definition."""

    name: str
    priority: int
    recipes: List[Recipe]


class SuiteValidator:  # pylint:disable=too-few-public-methods
    """Validate ETOS suite definitions to make sure they are executable."""

    logger = logging.getLogger(__name__)

    def __init__(self, params):
        """Initialize validator.

        :param params: Parameter instance.
        :type params: :obj:`etos_api.lib.params.Params`
        """
        self.params = params

    def _download_suite(self):
        """Attempt to download suite."""
        try:
            suite = requests.get(self.params.test_suite)
            suite.raise_for_status()
        except Exception as exception:  # pylint:disable=broad-except
            raise AssertionError(
                "Unable to download suite from %r" % self.params.test_suite
            ) from exception
        return suite.json()

    def validate(self):
        """Validate the ETOS suite definition.

        :raises ValidationError: If the suite did not validate.
        """
        downloaded_suite = self._download_suite()
        assert Suite(**downloaded_suite)
