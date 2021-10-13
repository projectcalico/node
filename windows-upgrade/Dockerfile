# Copyright (c) 2021 Tigera, Inc. All rights reserved.
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
ARG WINDOWS_VERSION
FROM --platform=linux/amd64 alpine:latest as base

# We can't RUN commands in the Windows image build.
# Create dirs we need that we copy over in the final stage.
RUN mkdir /CalicoUpgrade /CalicoBin

ADD bin/* /CalicoBin/

FROM mcr.microsoft.com/windows/servercore:${WINDOWS_VERSION}

ENV PATH="C:\Windows\System32\WindowsPowerShell\v1.0\;C:\Windows\system32;C:\Windows;"

COPY --from=base /CalicoUpgrade /CalicoUpgrade
COPY --from=base /CalicoBin /CalicoBin

ENTRYPOINT ["powershell", "-Command", "cp c:\\CalicoBin\\* c:\\CalicoUpgrade ; ls c:\\CalicoUpgrade ; ping -t localhost | out-null"]
