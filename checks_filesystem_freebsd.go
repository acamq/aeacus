//go:build freebsd

package main

func (c cond) FileOwner() (bool, error) {
	c.requireArgs("Path", "User", "Group")
	return unixFileOwner(c.Path, c.User, c.Group)
}

func (c cond) PermissionIs() (bool, error) {
	c.requireArgs("Path", "Value")
	return unixPermissionIs(c.Path, c.Value)
}
