//go:build freebsd

package main

import (
	"fmt"
	"testing"
)

func TestFreeBSDFileOwnerResolvesUserAndGroupIndependently(t *testing.T) {
	path, uid, gid := ownedUnixFixture(t)
	setUnixIdentityFixture(t,
		fmt.Sprintf("task-owner:x:%d:%d::/:/bin/sh\n", uid, nextUnixID(gid)),
		fmt.Sprintf("release-engineering:x:%d:\n", gid),
	)
	got, err := (cond{Path: path, User: "task-owner", Group: "release-engineering"}).FileOwner()
	if err != nil || !got {
		t.Fatalf("FreeBSD FileOwner explicit fields = (%v, %v), want (true, nil)", got, err)
	}
}

func TestFreeBSDPermissionIsUsesSharedParser(t *testing.T) {
	path, _, _ := ownedUnixFixture(t)
	got, err := (cond{Path: path, Value: "?rw-------"}).PermissionIs()
	if err != nil || !got {
		t.Fatalf("FreeBSD PermissionIs symbolic value = (%v, %v), want (true, nil)", got, err)
	}
}
