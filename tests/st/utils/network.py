# Copyright (c) 2015-2016 Tigera, Inc. All rights reserved.
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
import logging
import os

logger = logging.getLogger(__name__)

global_networking = None
NETWORKING_CNI = "cni"


def global_setting():
    global global_networking
    if global_networking is None:
        global_networking = os.getenv("ST_NETWORKING")
        if global_networking:
            assert global_networking == NETWORKING_CNI
        else:
            global_networking = NETWORKING_CNI
    return global_networking


class DummyNetwork(object):
    def __init__(self, name):
        self.name = name 
        self.network = name
        self.deleted = False
    def delete(self, host=None):
        pass
    def disconnect(self, host, container):
        pass
    def __str__(self):
        return self.name
