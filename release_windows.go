package main

// writeDesktopFiles writes default scoring engine files to the desktop.
func writeDesktopFiles() error {
	firefoxBinary := `C:\Program Files\Mozilla Firefox\firefox.exe`
	info("Writing ScoringReport.html shortcut to Desktop...")
	cmdString := `$WshShell = New-Object -comObject WScript.Shell; $Shortcut = $WshShell.CreateShortcut("C:\Users\` + conf.User + `\Desktop\ScoringReport.lnk"); $Shortcut.TargetPath = "` + firefoxBinary + `"; $Shortcut.Arguments = "C:\aeacus\assets\ScoringReport.html"; $Shortcut.Save()`
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}
	info("Writing ReadMe.html shortcut to Desktop...")
	cmdString = `$WshShell = New-Object -comObject WScript.Shell; $Shortcut = $WshShell.CreateShortcut("C:\Users\` + conf.User + `\Desktop\ReadMe.lnk"); $Shortcut.TargetPath = "` + firefoxBinary + `"; $Shortcut.Arguments = "C:\aeacus\assets\ReadMe.html"; $Shortcut.Save()`
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}
	info("Creating or emptying TeamID.txt file...")
	cmdString = "echo 'YOUR-TEAMID-HERE' > C:\\aeacus\\TeamID.txt"
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}
	info("Changing Permissions of TeamID...")
	powershellPermission := `
	$ACL = Get-ACL C:\aeacus\TeamID.txt
	$ACL.SetOwner([System.Security.Principal.NTAccount] $env:USERNAME)
	Set-Acl -Path C:\aeacus\TeamID.txt -AclObject $ACL
	`
	if err := runReleaseCommands(powershellPermission); err != nil {
		return err
	}
	info("Writing TeamID shortcut to Desktop...")
	cmdString = `$WshShell = New-Object -comObject WScript.Shell; $Shortcut = $WshShell.CreateShortcut("C:\Users\` + conf.User + `\Desktop\TeamID.lnk"); $Shortcut.TargetPath = "C:\aeacus\phocus.exe"; $Shortcut.Arguments = "-i yes"; $Shortcut.Save()`
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}

	// domain compatibility? doubt
	return nil
}

// configureAutologin allows the current user to log in automatically.
func configureAutologin() error {
	info("Setting Up autologin for " + conf.User + "...")
	powershellAutoLogin := `
	function Test-RegistryValue {

		param (

		 [parameter(Mandatory=$true)]
		 [ValidateNotNullOrEmpty()]$Path,

		[parameter(Mandatory=$true)]
		 [ValidateNotNullOrEmpty()]$Value
		)

		try {

		Get-ItemProperty -Path $Path | Select-Object -ExpandProperty $Value -ErrorAction Stop | Out-Null
		 return $true
		 }

		catch {

		return $false

		}

	}
	$RegPath1Exists = Test-RegistryValue -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Value "DefaultUsername"
	if ($RegPath1Exists -eq $false) {
		New-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -name "DefaultUsername" -Value $env:USERNAME -type String
	}
	elseif ($RegPath1Exists -eq $true) {
		Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -name "DefaultUsername" -Value $env:USERNAME -type String
	}

	$RegPath2Exists = Test-RegistryValue -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -Value "AutoAdminLogon"
	if ($RegPath2Exists -eq $false) {
		New-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -name "AutoAdminLogon" -Value 1 -type String
	}
	elseif ($RegPath2Exists -eq $true) {
		Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" -name "AutoAdminLogon" -Value 1 -type String
	}
	`
	return runReleaseCommands(powershellAutoLogin)
}

// installFont installs the Raleway font for ID Prompt.
func installFont() error {
	info("Installing Raleway font for ID Prompt...")
	powershellFontInstall := `
	$SourceDir   = "C:\aeacus\assets\fonts\Raleway"
	$Source      = "C:\aeacus\assets\fonts\Raleway\*"
	$Destination = (New-Object -ComObject Shell.Application).Namespace(0x14)
	$TempFolder  = "C:\Windows\Temp\Fonts"

	# Create the source directory if it doesn't already exist
	New-Item -ItemType Directory -Force -Path $SourceDir | Out-Null

	New-Item $TempFolder -Type Directory -Force | Out-Null

	Get-ChildItem -Path $Source -Include '*.ttf','*.ttc','*.otf' -Recurse | ForEach {
		If (-not(Test-Path "C:\Windows\Fonts\$($_.Name)")) {

			$Font = "$TempFolder\$($_.Name)"

			# Copy font to local temporary folder
			Copy-Item $($_.FullName) -Destination $TempFolder

			# Install font
			$Destination.CopyHere($Font,0x10)

			# Delete temporary copy of font
			Remove-Item $Font -Force
		}
	}
	`
	return runReleaseCommands(powershellFontInstall)
}

// installService installs the Aeacus service on Windows.
func installService() error {
	info("Installing service with sc.exe...")
	cmdString := `sc.exe create CSSClient binPath= "C:\aeacus\phocus.exe" start= "auto" DisplayName= "CSSClient"`
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}
	info("Setting service description...")
	cmdString = `sc.exe description CSSClient "This is Aeacus's Competition Scoring System client. Don't stop or mess with this unless you want to not get points, and maybe have your registry deleted."`
	if err := runReleaseCommands(cmdString); err != nil {
		return err
	}
	info("Setting up TeamID scheduled task...")
	idTaskCreate := `
	$action = New-ScheduledTaskAction -Execute "C:\aeacus\phocus.exe" -Argument "-i yes"
	$trigger = New-ScheduledTaskTrigger -AtLogon
	$principal = New-ScheduledTaskPrincipal -GroupId "BUILTIN\Administrators" -RunLevel Highest
	Register-ScheduledTask -TaskName "TeamID" -Description "Scheduled Task to ensure Aeacus TeamID prompt is displayed when needed" -Action $action -Trigger $trigger -Principal $principal
	`
	serviceTaskCreate := `
	$action = New-ScheduledTaskAction -Execute "net.exe" -Argument "start CSSClient"
	$trigger = New-ScheduledTaskTrigger -AtLogon
	$principal = New-ScheduledTaskPrincipal -GroupId "BUILTIN\Administrators" -RunLevel Highest
	Register-ScheduledTask -TaskName "CSSClient" -Description "Scheduled Task to ensure the CSSClient service remains up" -Action $action -Trigger $trigger -Principal $principal
	`
	if err := runReleaseCommands(idTaskCreate, serviceTaskCreate); err != nil {
		return err
	}

	addExclusions := `
	Add-MpPreference -ExclusionPath "C:\aeacus\phocus.exe"
 	Add-MpPreference -ExclusionPath "C:\aeacus\"
  	`
	return runReleaseCommands(addExclusions)
}

// cleanUp clears out sensitive files left behind by image developers or the
// scoring engine.
func cleanUp() error {
	info("Removing scoring.conf and ReadMe.conf...")
	if err := runReleaseCommands(
		"Remove-Item -Force C:\\aeacus\\scoring.conf",
		"Remove-Item -Path 'C:\\aeacus\\[R|r]*.conf' -Force",
	); err != nil {
		return err
	}
	info("Removing previous.txt...")
	if err := runReleaseCommands("Remove-Item -Force C:\\aeacus\\previous.txt"); err != nil {
		return err
	}
	if !ask("Do you want to remove cache and history files from this machine?") {
		return nil
	}
	info("Emptying recycle bin...")
	if err := runReleaseCommands("Clear-RecycleBin -Force"); err != nil {
		return err
	}
	info("Clearing recently used...")
	if err := runReleaseCommands("Remove-Item -Force '${env:USERPROFILE}\\AppData\\Roaming\\Microsoft\\Windows\\Recent‌​*.lnk'"); err != nil {
		return err
	}
	info("Clearing run.exe command history...")
	clearRunScript := `$path = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\RunMRU"
	$arr = (Get-Item -Path $path).Property
	foreach($item in $arr)
	{
	   if($item -ne "MRUList")
	   {
		 Remove-ItemProperty -Path $path -Name $item -ErrorAction SilentlyContinue
	   }
	}`
	if err := runReleaseCommands(clearRunScript); err != nil {
		return err
	}
	info("Removing Command History for Powershell")
	if err := runReleaseCommands("Remove-Item (Get-PSReadlineOption).HistorySavePath"); err != nil {
		return err
	}
	warn("Done with automatic cleanup! You need to remove aeacus.exe manually. The only things you need in the C:\\aeacus directory is phocus, scoring.dat, TeamID.txt, and the assets directory.")
	return nil
}
