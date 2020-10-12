# Copyright (c) 2020 Tigera, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# We require the 64-bit version of Powershell, which should live at the following path.
$powerShellPath = "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
$baseDir = "$PSScriptRoot\.."
$NSSMPath = "$baseDir\nssm-2.24\win64\nssm.exe"
$kubePath = "c:\k"

function Install-KubeletService()
{
    Write-Host "Installing kubelet service..."

    # Ensure our service file can run.
    Unblock-File $baseDir\kubernetes\kubelet-service.ps1

    & $NSSMPath install kubelet $powerShellPath
    & $NSSMPath set kubelet AppParameters $baseDir\kubernetes\kubelet-service.ps1
    & $NSSMPath set kubelet AppDirectory $baseDir
    & $NSSMPath set kubelet DisplayName "kubelet service"
    & $NSSMPath set kubelet Description "Kubenetes kubelet node agent."

    # Configure it to auto-start by default.
    & $NSSMPath set kubelet Start SERVICE_AUTO_START
    & $NSSMPath set kubelet ObjectName LocalSystem
    & $NSSMPath set kubelet Type SERVICE_WIN32_OWN_PROCESS

    # Throttle process restarts if restarts in under 1500ms.
    & $NSSMPath set kubelet AppThrottle 1500

    & $NSSMPath set kubelet AppStdout $kubePath\kubelet.out.log
    & $NSSMPath set kubelet AppStderr $kubePath\kubelet.err.log

    # Configure online file rotation.
    & $NSSMPath set kubelet AppRotateFiles 1
    & $NSSMPath set kubelet AppRotateOnline 1
    # Rotate once per day.
    & $NSSMPath set kubelet AppRotateSeconds 86400
    # Rotate after 10MB.
    & $NSSMPath set kubelet AppRotateBytes 10485760

    Write-Host "Done installing kubelet service."
}

Install-KubeletService
