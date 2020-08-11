# Copyright (c) 2018-2020 Tigera, Inc. All rights reserved.
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
$baseDir = "$PSScriptRoot\..\.."
$NSSMPath = "$baseDir\nssm-2.24\win64\nssm.exe"

function fileIsMissing($path)
{
    return (("$path" -EQ "") -OR (-NOT(Test-Path "$path")))
}

function Test-CalicoConfiguration()
{
    Write-Host "Validating configuration..."
    if (!$env:CNI_BIN_DIR)
    {
        throw "Config not loaded?."
    }
    if ($env:CALICO_NETWORKING_BACKEND -EQ "vxlan") {
        if (fileIsMissing($env:CNI_BIN_DIR))
        {
            throw "CNI binary directory $env:CNI_BIN_DIR doesn't exist.  Please create it and ensure kubelet " +  `
                    "is configured with matching --cni-bin-dir."
        }
        if (fileIsMissing($env:CNI_CONF_DIR))
        {
            throw "CNI config directory $env:CNI_CONF_DIR doesn't exist.  Please create it and ensure kubelet " +  `
                    "is configured with matching --cni-conf-dir."
        }
    }
    if ($env:CALICO_NETWORKING_BACKEND -EQ "vxlan" -AND $env:CNI_IPAM_TYPE -NE "calico-ipam") {
        throw "Calico VXLAN requires IPAM type calico-ipam, not $env:CNI_IPAM_TYPE."
    }
    if ($env:CALICO_DATASTORE_TYPE -EQ "kubernetes")
    {
        if (fileIsMissing($env:KUBECONFIG))
        {
            throw "kubeconfig file $env:KUBECONFIG doesn't exist.  Please update the configuration to match. " +  `
                    "the location of your kubeconfig file."
        }
    }
    elseif ($env:CALICO_DATASTORE_TYPE -EQ "etcdv3")
    {
        if (("$env:ETCD_ENDPOINTS" -EQ "") -OR ("$env:ETCD_ENDPOINTS" -EQ "<your etcd endpoints>"))
        {
            throw "Etcd endpoint not set, please update the configuration."
        }
        if (("$env:ETCD_KEY_FILE" -NE "") -OR ("$env:ETCD_CERT_FILE" -NE "") -OR ("$env:ETCD_CA_CERT_FILE" -NE ""))
        {
            if (fileIsMissing($env:ETCD_KEY_FILE))
            {
                throw "Some etcd TLS parameters are configured but etcd key file was not found."
            }
            if (fileIsMissing($env:ETCD_CERT_FILE))
            {
                throw "Some etcd TLS parameters are configured but etcd certificate file was not found."
            }
            if (fileIsMissing($env:ETCD_CA_CERT_FILE))
            {
                throw "Some etcd TLS parameters are configured but etcd CA certificate file was not found."
            }
        }
    }
    else
    {
        throw "Please set datastore type to 'etcdv3' or 'kubernetes'; current value: $env:CALICO_DATASTORE_TYPE."
    }
}

function Install-CNIPlugin()
{
    Write-Host "Copying CNI binaries into place."
    cp "$baseDir\cni\*.exe" "$env:CNI_BIN_DIR"

    $cniConfFile = $env:CNI_CONF_DIR + "\" + $env:CNI_CONF_FILENAME
    Write-Host "Writing CNI configuration to $cniConfFile."
    $nodeNameFile = "$baseDir\nodename".replace('\', '\\')
    $etcdKeyFile = "$env:ETCD_KEY_FILE".replace('\', '\\')
    $etcdCertFile = "$env:ETCD_CERT_FILE".replace('\', '\\')
    $etcdCACertFile = "$env:ETCD_CA_CERT_FILE".replace('\', '\\')
    $kubeconfigFile = "$env:KUBECONFIG".replace('\', '\\')
    $mode = ""
    if ($env:CALICO_NETWORKING_BACKEND -EQ "vxlan") {
        $mode = "vxlan"
    }

    $dnsIPs=$env:DNS_NAME_SERVERS.Split(",")
    $ipList = @()
    foreach ($ip in $dnsIPs) {
        $ipList += "`"$ip`""
    }
    $dnsIPList=($ipList -join ",").TrimEnd(',')

    (Get-Content "$baseDir\cni.conf.template") | ForEach-Object {
        $_.replace('__NODENAME_FILE__', $nodeNameFile).
                replace('__KUBECONFIG__', $kubeconfigFile).
                replace('__K8S_SERVICE_CIDR__', $env:K8S_SERVICE_CIDR).
                replace('__DNS_NAME_SERVERS__', $dnsIPList).
                replace('__DATASTORE_TYPE__', $env:CALICO_DATASTORE_TYPE).
                replace('__ETCD_ENDPOINTS__', $env:ETCD_ENDPOINTS).
                replace('__ETCD_KEY_FILE__', $etcdKeyFile).
                replace('__ETCD_CERT_FILE__', $etcdCertFile).
                replace('__ETCD_CA_CERT_FILE__', $etcdCACertFile).
                replace('__IPAM_TYPE__', $env:CNI_IPAM_TYPE).
                replace('__MODE__', $mode).
                replace('__VNI__', $env:VXLAN_VNI).
                replace('__MAC_PREFIX__', $env:VXLAN_MAC_PREFIX)
    } | Set-Content "$cniConfFile"
    Write-Host "Wrote CNI configuration."
}

function Remove-CNIPlugin()
{
    $cniConfFile = $env:CNI_CONF_DIR + "\" + $env:CNI_CONF_FILENAME
    Write-Host "Removing $cniConfFile and Calico binaries."
    rm $cniConfFile
    rm "$env:CNI_BIN_DIR/calico*.exe"
}

function Install-NodeService()
{
    Write-Host "Installing node startup service..."

    ensureRegistryKey

    # Ensure our service file can run.
    Unblock-File $baseDir\node\node-service.ps1

    & $NSSMPath install CalicoNode $powerShellPath
    & $NSSMPath set CalicoNode AppParameters $baseDir\node\node-service.ps1
    & $NSSMPath set CalicoNode AppDirectory $baseDir
    & $NSSMPath set CalicoNode DisplayName "Calico Windows Startup"
    & $NSSMPath set CalicoNode Description "Calico Windows Startup, configures Calico datamodel resources for this node."

    # Configure it to auto-start by default.
    & $NSSMPath set CalicoNode Start SERVICE_AUTO_START
    & $NSSMPath set CalicoNode ObjectName LocalSystem
    & $NSSMPath set CalicoNode Type SERVICE_WIN32_OWN_PROCESS

    # Throttle process restarts if Felix restarts in under 1500ms.
    & $NSSMPath set CalicoNode AppThrottle 1500

    # Create the log directory if needed.
    if (-Not(Test-Path "$env:CALICO_LOG_DIR"))
    {
        write "Creating log directory."
        md -Path "$env:CALICO_LOG_DIR"
    }
    & $NSSMPath set CalicoNode AppStdout $env:CALICO_LOG_DIR\calico-node.log
    & $NSSMPath set CalicoNode AppStderr $env:CALICO_LOG_DIR\calico-node.err.log

    # Configure online file rotation.
    & $NSSMPath set CalicoNode AppRotateFiles 1
    & $NSSMPath set CalicoNode AppRotateOnline 1
    # Rotate once per day.
    & $NSSMPath set CalicoNode AppRotateSeconds 86400
    # Rotate after 10MB.
    & $NSSMPath set CalicoNode AppRotateBytes 10485760

    Write-Host "Done installing startup service."
}

function Remove-NodeService()
{
    & $NSSMPath remove CalicoNode confirm
}

function Install-FelixService()
{
    Write-Host "Installing Felix service..."

    # Ensure our service file can run.
    Unblock-File $baseDir\felix\felix-service.ps1

    # We run Felix via a wrapper script to make it easier to update env vars.
    & $NSSMPath install CalicoFelix $powerShellPath
    & $NSSMPath set CalicoFelix AppParameters $baseDir\felix\felix-service.ps1
    & $NSSMPath set CalicoFelix AppDirectory $baseDir
    & $NSSMPath set CalicoFelix DependOnService "CalicoNode"
    & $NSSMPath set CalicoFelix DisplayName "Calico Windows Agent"
    & $NSSMPath set CalicoFelix Description "Calico Windows Per-host Agent, Felix, provides network policy enforcement for Kubernetes."

    # Configure it to auto-start by default.
    & $NSSMPath set CalicoFelix Start SERVICE_AUTO_START
    & $NSSMPath set CalicoFelix ObjectName LocalSystem
    & $NSSMPath set CalicoFelix Type SERVICE_WIN32_OWN_PROCESS

    # Throttle process restarts if Felix restarts in under 1500ms.
    & $NSSMPath set CalicoFelix AppThrottle 1500

    # Create the log directory if needed.
    if (-Not(Test-Path "$env:CALICO_LOG_DIR"))
    {
        write "Creating log directory."
        md -Path "$env:CALICO_LOG_DIR"
    }
    & $NSSMPath set CalicoFelix AppStdout $env:CALICO_LOG_DIR\calico-felix.log
    & $NSSMPath set CalicoFelix AppStderr $env:CALICO_LOG_DIR\calico-felix.err.log

    # Configure online file rotation.
    & $NSSMPath set CalicoFelix AppRotateFiles 1
    & $NSSMPath set CalicoFelix AppRotateOnline 1
    # Rotate once per day.
    & $NSSMPath set CalicoFelix AppRotateSeconds 86400
    # Rotate after 10MB.
    & $NSSMPath set CalicoFelix AppRotateBytes 10485760

    Write-Host "Done installing Felix service."
}

function Remove-FelixService() {
    & $NSSMPath remove CalicoFelix confirm
}

function Wait-ForManagementIP($NetworkName)
{
    while ((Get-HnsNetwork | ? Name -EQ $NetworkName).ManagementIP -EQ $null)
    {
        Write-Host "Waiting for management IP to appear on network $NetworkName..."
        Start-Sleep 1
    }
    return (Get-HnsNetwork | ? Name -EQ $NetworkName).ManagementIP
}

function Get-LastBootTime()
{
    $bootTime = (Get-WmiObject win32_operatingsystem | select @{LABEL='LastBootUpTime';EXPRESSION={$_.lastbootuptime}}).LastBootUpTime
    if (($bootTime -EQ $null) -OR ($bootTime.length -EQ 0))
    {
        throw "Failed to get last boot time"
    }
    return $bootTime
}

$softwareRegistryKey = "HKLM:\Software\Tigera"
$calicoRegistryKey = $softwareRegistryKey + "\Calico"

function ensureRegistryKey()
{
    if (! (Test-Path $softwareRegistryKey))
    {
        New-Item $softwareRegistryKey
    }
    if (! (Test-Path $calicoRegistryKey))
    {
        New-Item $calicoRegistryKey
    }
}

function Get-StoredLastBootTime()
{
    try
    {
        return (Get-ItemProperty $calicoRegistryKey -ErrorAction Ignore).LastBootTime
    }
    catch
    {
        $PSItem.Exception.Message
    }
}

function Set-StoredLastBootTime($lastBootTime)
{
    ensureRegistryKey

    return Set-ItemProperty $calicoRegistryKey -Name LastBootTime -Value $lastBootTime
}

function Wait-ForCalicoInit()
{
    Write-Host "Waiting for Calico initialisation to finish..."
    $Stored=Get-StoredLastBootTime
    $Current=Get-LastBootTime
    while ($Stored -NE $Current) {
        Write-Host "Waiting for Calico initialisation to finish...StoredLastBootTime $Stored, CurrentLastBootTime $Current"
        Start-Sleep 1

        $Stored=Get-StoredLastBootTime
        $Current=Get-LastBootTime
    }
    Write-Host "Calico initialisation finished."
}

Export-ModuleMember -Function 'Test-*'
Export-ModuleMember -Function 'Install-*'
Export-ModuleMember -Function 'Remove-*'
Export-ModuleMember -Function 'Wait-*'
Export-ModuleMember -Function 'Get-*'
Export-ModuleMember -Function 'Set-*'
