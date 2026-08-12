package main

import (
	"errors"
	"strings"
	"syscall"
)

func (c cond) AutoCheckUpdatesEnabled() (bool, error) {
	result, err := cond{
		Path:  "/etc/apt/apt.conf.d/",
		Value: `(?i)^\s*APT::Periodic::Update-Package-Lists\s+"1"\s*;\s*$`,
		regex: true,
	}.DirContains()
	// If /etc/apt/ does not exist, try dnf (RHEL)
	if err != nil {
		autoConf, err := cond{
			Path: "/etc/dnf/automatic.conf",
		}.PathExists()
		if err != nil {
			return false, err
		}
		if autoConf {
			applyUpdates, err := cond{
				Path:  "/etc/dnf/automatic.conf",
				Value: `(?i)^\s*apply_updates\s*=\s*(1|on|yes|true)`,
				regex: true,
			}.FileContains()
			if err != nil {
				return false, err
			}

			autoTimer, err := cond{
				Path: "/etc/systemd/system/timers.target.wants/dnf-automatic.timer",
			}.PathExists()
			if err != nil {
				return false, err
			}

			if applyUpdates && autoTimer {
				return true, nil
			}

			autoInstallTimer, err := cond{
				Path: "/etc/systemd/system/timers.target.wants/dnf-automatic-install.timer",
			}.PathExists()
			if err != nil {
				return false, err
			}
			return autoInstallTimer, nil
		}

	}
	return result, err
}

func (c cond) FirewallUp() (bool, error) {
	result, err := cond{
		Path:  "/etc/ufw/ufw.conf",
		Value: `^\s*ENABLED=yes\s*$`,
		regex: true,
	}.FileContains()
	if err != nil {
		// If ufw.conf does not exist, check firewalld status (RHEL)
		return cond{
			Name: "firewalld",
		}.ServiceUp()
	}
	return result, err
}

func (c cond) GuestDisabledLDM() (bool, error) {
	guestStr := `\s*allow-guest\s*=\s*false`
	result, err := cond{
		Path:  "/usr/share/lightdm/lightdm.conf.d/",
		Value: guestStr,
		regex: true,
	}.DirContains()
	if !result {
		return cond{
			Path:  "/etc/lightdm/",
			Value: guestStr,
			regex: true,
		}.DirContains()
	}
	return result, err
}

func (c cond) KernelVersion() (bool, error) {
	c.requireArgs("Value")
	utsname := syscall.Utsname{}
	err := syscall.Uname(&utsname)
	releaseUint := []byte{}
	for i := 0; i < 65; i++ {
		if utsname.Release[i] == 0 {
			break
		}
		releaseUint = append(releaseUint, uint8(utsname.Release[i]))
	}
	debug("System uname value is", string(releaseUint), "and our value is", c.Value)
	return string(releaseUint) == c.Value, err
}

func (c cond) PasswordChanged() (bool, error) {
	c.requireArgs("User", "Value")
	fileContent, err := readFile("/etc/shadow")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(fileContent, "\n") {
		if strings.Contains(line, c.User+":") {
			if strings.Contains(line, c.User+":"+c.Value) {
				debug("Exact value found in /etc/shadow for user", c.User+":", line)
				return false, nil
			}
			debug("Differing value found in /etc/shadow for user", c.User+":", line)
			return true, nil
		}
	}
	return false, errors.New("user not found")
}

func (c cond) FileOwner() (bool, error) {
	c.requireArgs("Path", "Name")
	return unixFileOwner(c.Path, c.Name, "")
}

func (c cond) PermissionIs() (bool, error) {
	c.requireArgs("Path", "Value")
	return unixPermissionIs(c.Path, c.Value)
}

func (c cond) UserExists() (bool, error) {
	c.requireArgs("User")
	return (unixIdentityDatabase{reader: unixIdentityFiles}).UserExists(c.User)
}

func (c cond) UserInGroup() (bool, error) {
	c.requireArgs("User", "Group")
	return (unixIdentityDatabase{reader: unixIdentityFiles}).UserInGroup(c.User, c.Group)
}
