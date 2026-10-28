# Only for disposable hosted runners; never changes a developer's firewall.
function New-InstallerTestNetworkIsolation {
 param([Parameter(Mandatory=$true)][string]$Name,[Parameter(Mandatory=$true)][string]$Program)
 if ($env:GITHUB_ACTIONS -ne 'true') { throw 'Installer network isolation requires an isolated GitHub runner.' }
 New-NetFirewallRule -Name $Name -DisplayName $Name -Direction Outbound `
  -Program $Program -Action Block -Profile Any -RemoteAddress Internet
}
