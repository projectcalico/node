# Copyright (c) 2021 Tigera, Inc. All rights reserved.
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

$rootDir = "c:\CalicoWindows"
$zipFile = "c:\calico-windows.zip"

$date = Get-Date -UFormat "%Y-%m-%d"
$logFile = "c:\calico-upgrade.$date.log"

function Log {
    param ([String]$value)
    $stamp = Get-Date -UFormat "%Y-%m-%d %R:%S"
    Add-content -Path $logFile -Value "$stamp [INFO] $value"
}

Log "Starting calico upgrade"

Log "Files in c:\CalicoUpgrade:"
$files = ls c:\CalicoUpgrade | Out-String
Log $files

Log "Copying installation zip file"
cp $PSScriptRoot\*.zip $zipFile

Log "Starting installation script..."
& $PSScriptRoot\install-calico-windows.ps1 *>> $logFile

Log "Cleaning up"
Remove-Item -Path $PSScriptRoot\*.ps1
Remove-Item -Path $PSScriptRoot\*.zip
$oldCalicoNode = "$rootDir\calico-node.exe.to-be-replaced"
if (Test-Path $oldCalicoNode)
{
    Remove-Item -Path $oldCalicoNode
}

Log "Finished upgrade"
