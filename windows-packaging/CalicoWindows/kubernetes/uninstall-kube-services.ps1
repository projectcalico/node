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

param
(
     [string][parameter(Mandatory=$false)]$service
)

$baseDir = "$PSScriptRoot\.."
$NSSMPath = "$baseDir\nssm-2.24\win64\nssm.exe"

$ErrorActionPreference = 'SilentlyContinue'

if (($service -ne "") -and ($service -notin "kubelet", "kube-proxy"))
{
    Write-Host "Invalid -service value. Valid values are: 'kubelet' or 'kube-proxy'"
    Exit
}

if ($service -eq "")
{
    Write-Host "Stopping kubelet kube-proxy services if they are running..."
    Stop-Service kubelet
    Stop-Service kube-proxy
    
    & $NSSMPath remove kube-proxy confirm
    & $NSSMPath remove kubelet confirm
    Write-Host "Done"
}
elseif ($service -eq "kubelet")
{
    Write-Host "Stopping kubelet service if it is running..."
    Stop-Service kubelet
    & $NSSMPath remove kubelet confirm
    Write-Host "Done"
}
else
{
    Write-Host "Stopping kube-proxy service if it is running..."
    Stop-Service kube-proxy
    & $NSSMPath remove kube-proxy confirm
    Write-Host "Done"
}
