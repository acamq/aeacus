package main

import (
	"errors"
	"fmt"
)

// writeDesktopFiles creates TeamID.txt and its shortcut, as well as links
// to the ScoringReport, ReadMe, and other needed files.
func writeDesktopFiles() error {
	info("Creating or emptying TeamID.txt...")
	if err := runReleaseCommands(
		"echo 'YOUR-TEAMID-HERE' > "+dirPath+"TeamID.txt",
		"chmod 666 "+dirPath+"TeamID.txt",
		"chown "+conf.User+":"+conf.User+" "+dirPath+"TeamID.txt",
	); err != nil {
		return err
	}
	info("Writing shortcuts to Desktop...")
	return runReleaseCommands(
		"mkdir -p /home/"+conf.User+"/Desktop/",
		"cp "+dirPath+"misc/desktop/*.desktop /home/"+conf.User+"/Desktop/",
		"chmod +x /home/"+conf.User+"/Desktop/*.desktop",
		"chown "+conf.User+":"+conf.User+" /home/"+conf.User+"/Desktop/*",
	)
}

// configureAutologin configures the auto-login capability for LightDM and
// GDM3, so that the image automatically logs in to the main user's account
// on boot.
func configureAutologin() error {
	lightdm, err := cond{Path: "/usr/share/lightdm"}.PathExists()
	if err != nil {
		return fmt.Errorf("detect LightDM: %w", err)
	}
	gdm, err := cond{Path: "/etc/gdm3/"}.PathExists()
	if err != nil {
		return fmt.Errorf("detect GDM3: %w", err)
	}
	if lightdm {
		info("LightDM detected for autologin.")
		return runReleaseCommands(`echo "autologin-user=` + conf.User + `" >> /usr/share/lightdm/lightdm.conf.d/50-ubuntu.conf`)
	} else if gdm {
		info("GDM3 detected for autologin.")
		return runReleaseCommands(`echo -e "AutomaticLoginEnable=True\nAutomaticLogin=` + conf.User + `" >> /etc/gdm3/daemon.conf`)
	}
	return errors.New("supported display manager not found")
}

// installFont is skipped for Linux.
func installFont() error {
	info("Skipping font install for Linux...")
	return nil
}

// installService for Linux installs and starts the CSSClient init.d service.
func installService() error {
	info("Installing service...")
	return runReleaseCommands(
		"cp "+dirPath+"misc/dev/CSSClient /etc/init.d/",
		"chmod +x /etc/init.d/CSSClient",
		"systemctl enable CSSClient",
		"systemctl start CSSClient",
	)
}

// cleanUp for Linux is primarily focused on removing cached files, history,
// and other pieces of forensic evidence. It also removes the non-required
// files in the aeacus directory.
func cleanUp() error {
	findPaths := "/bin /etc /home /opt /root /sbin /srv /usr /mnt /var"

	info("Changing perms to 755 in " + dirPath + "...")
	if err := runReleaseCommands("chmod 755 -R " + dirPath); err != nil {
		return err
	}

	info("Removing aeacus binary...")
	if err := runReleaseCommands("rm " + dirPath + "aeacus"); err != nil {
		return err
	}

	info("Removing scoring.conf...")
	if err := runReleaseCommands("rm " + dirPath + "scoring.conf*"); err != nil {
		return err
	}

	info("Removing other setup files...")
	if err := runReleaseCommands(
		"rm -rf "+dirPath+"misc/",
		"find "+dirPath+" -name '[R|r]*.conf' -type f -delete",
		"rm -rf "+dirPath+"README.md",
		"rm -rf "+dirPath+".git",
		"rm -rf "+dirPath+".github",
		"rm -rf "+dirPath+"*.go",
		"rm -rf "+dirPath+"Makefile",
		"rm -rf "+dirPath+"go.*",
		"rm -rf "+dirPath+"*.exe",
		"rm -rf "+dirPath+"docs",
	); err != nil {
		return err
	}

	if !ask("Do you want to remove cache and log files, overwrite timestamps, and remove other forensic data from this machine? This may impact data used for your forensic questions!") {
		return nil
	}

	info("Removing .viminfo and .swp files...")
	cleanupCommands := linuxForensicCleanupCommands()
	if err := runReleaseCommands(cleanupCommands[:2]...); err != nil {
		return err
	}

	info("Symlinking .bash_history and .zsh_history to /dev/null...")
	if err := runReleaseCommands(
		`find `+findPaths+` -iname '*.bash_history' -exec ln -sf /dev/null {} \;`,
		`find `+findPaths+` -name '.zsh_history' -exec ln -sf /dev/null {} \;`,
	); err != nil {
		return err
	}

	info("Removing .mysql_history...")
	if err := runReleaseCommands(`find ` + findPaths + ` -name '.mysql_history' -exec rm {} \;`); err != nil {
		return err
	}

	info("Removing .local files...")
	if err := runReleaseCommands("rm -rf /root/.local /home/*/.local/"); err != nil {
		return err
	}

	info("Removing cache...")
	if err := runReleaseCommands("rm -rf /root/.cache /home/*/.cache/"); err != nil {
		return err
	}

	info("Removing temp root and Desktop files...")
	if err := runReleaseCommands("rm -rf /root/*~ /home/*/Desktop/*~"); err != nil {
		return err
	}

	info("Removing crash and VMWare data...")
	if err := runReleaseCommands("rm -f /var/VMwareDnD/* /var/crash/*.crash"); err != nil {
		return err
	}

	info("Removing apt and dpkg logs...")
	if err := runReleaseCommands("rm -rf /var/log/apt/* /var/log/dpkg.log"); err != nil {
		return err
	}

	info("Removing logs (auth and syslog)...")
	if err := runReleaseCommands("rm -f /var/log/auth.log* /var/log/syslog*"); err != nil {
		return err
	}

	info("Removing initial package list...")
	if err := runReleaseCommands("rm -f /var/log/installer/initial-status.gz"); err != nil {
		return err
	}

	info("Installing BleachBit...")
	if err := runReleaseCommands(linuxBleachBitInstallCommands()...); err != nil {
		return err
	}

	info("Clearing Firefox cache and browsing history...")
	if err := runReleaseCommands(cleanupCommands[2:]...); err != nil {
		return err
	}

	info("Overwriting timestamps to obfuscate changes...")
	return runReleaseCommands(`find /etc /home /var -exec touch --date='2012-12-12 12:12' {} \; 2>/dev/null`)
}

func linuxForensicCleanupCommands() []string {
	findPaths := "/bin /etc /home /opt /root /sbin /srv /usr /mnt /var"
	return []string{
		"find " + findPaths + " -iname '*.viminfo*' -delete",
		"find " + findPaths + " -iname '*.swp' -delete",
		"bleachbit --clean firefox.url_history",
		"bleachbit --clean firefox.cache",
	}
}

func linuxBleachBitInstallCommands() []string {
	return []string{"apt-get install -y bleachbit"}
}
