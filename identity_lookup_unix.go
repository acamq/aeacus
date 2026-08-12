//go:build linux || freebsd

package main

func (d unixIdentityDatabase) lookupUser(name string) (unixUserRecord, error) {
	data, err := d.readPasswd()
	if err != nil {
		return unixUserRecord{}, err
	}
	users, err := parseUnixPasswd(data)
	if err != nil {
		return unixUserRecord{}, err
	}
	user, found := users[name]
	if !found {
		return unixUserRecord{}, &unixIdentityError{Kind: ErrUnixUserNotFound, Path: unixPasswdPath, Name: name}
	}
	return user, nil
}

func (d unixIdentityDatabase) lookupGroup(name string) (unixGroupRecord, error) {
	data, err := d.readGroup()
	if err != nil {
		return unixGroupRecord{}, err
	}
	groups, err := parseUnixGroup(data)
	if err != nil {
		return unixGroupRecord{}, err
	}
	group, found := groups[name]
	if !found {
		return unixGroupRecord{}, &unixIdentityError{Kind: ErrUnixGroupNotFound, Path: unixGroupPath, Name: name}
	}
	return group, nil
}
