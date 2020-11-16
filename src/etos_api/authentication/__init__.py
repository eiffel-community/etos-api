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
"""ETOS API authentication modules.

Authentication model.

1. User requests API without token.
2. User must provide authentication based on enabled :obj:`etos_api.authentication.authorize`
   model.
3. Authorization model provides a JWT access token to user.
4. User requests API with token.
5. Request successful.

This access token is available for limited time and after it has expired, the user
must authorize again.
"""
