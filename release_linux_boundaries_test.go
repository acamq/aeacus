//go:build linux && !phocus

package main

import (
	"errors"
	"os"
	"os/user"
	"reflect"
	"testing"
)

func TestCheckLinuxAdministratorReturnsLookupError(t *testing.T) {
	tests := []struct {
		name    string
		account *user.User
		err     error
	}{
		{name: "lookup error", err: errors.New("user lookup failed")},
		{name: "nil account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			administrator, err := checkLinuxAdministrator(func() (*user.User, error) {
				return tt.account, tt.err
			})

			if administrator {
				t.Fatal("administrator = true, want false")
			}
			if err == nil {
				t.Fatal("error = nil")
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want lookup cause", err)
			}
		})
	}
}

func TestCheckReleasePermissionsReturnsAdministratorErrorWithoutPanic(t *testing.T) {
	oldCheck := releaseAdministratorCheck
	t.Cleanup(func() { releaseAdministratorCheck = oldCheck })
	releaseAdministratorCheck = func() bool { return false }

	err := checkReleasePermissions()

	if !errors.Is(err, errReleaseAdministratorRequired) {
		t.Fatalf("error = %v, want administrator required", err)
	}
}

func TestCheckLinuxAdministratorParsesRootIdentity(t *testing.T) {
	tests := []struct {
		name              string
		uid               string
		wantAdministrator bool
		wantErr           bool
	}{
		{name: "root", uid: "0", wantAdministrator: true},
		{name: "non-root", uid: "1000"},
		{name: "malformed uid", uid: "root", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			administrator, err := checkLinuxAdministrator(func() (*user.User, error) {
				return &user.User{Uid: tt.uid}, nil
			})

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if administrator != tt.wantAdministrator {
				t.Fatalf("administrator = %v, want %v", administrator, tt.wantAdministrator)
			}
		})
	}
}

func TestLinuxCleanupCommandsKeepIndependentFailuresVisible(t *testing.T) {
	commands := linuxForensicCleanupCommands()
	want := []string{
		"find /bin /etc /home /opt /root /sbin /srv /usr /mnt /var -iname '*.viminfo*' -delete",
		"find /bin /etc /home /opt /root /sbin /srv /usr /mnt /var -iname '*.swp' -delete",
		"bleachbit --clean firefox.url_history",
		"bleachbit --clean firefox.cache",
	}

	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestLinuxCleanupStopsBeforeLaterIndependentCommand(t *testing.T) {
	sentinel := t.TempDir() + "/later-command"

	err := runReleaseCommands("exit 23", "touch "+sentinel)

	if err == nil {
		t.Fatal("runReleaseCommands() error = nil")
	}
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later cleanup command ran: %v", statErr)
	}
}

func TestLinuxCleanupInstallsBleachBitWithoutPackageMetadataRefresh(t *testing.T) {
	commands := linuxBleachBitInstallCommands()
	want := []string{"apt-get install -y bleachbit"}

	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestLinuxBleachBitFailurePreventsCacheCleanup(t *testing.T) {
	sentinel := t.TempDir() + "/cache-cleanup"
	commands := linuxForensicCleanupCommands()
	commands[2] = "exit 23"
	commands[3] = "touch " + sentinel

	err := runReleaseCommands(commands[2:]...)

	if err == nil {
		t.Fatal("runReleaseCommands() error = nil")
	}
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cache cleanup ran: %v", statErr)
	}
}
