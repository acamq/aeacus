package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveCapabilitiesMatrix(t *testing.T) {
	tests := []struct {
		name           string
		goos           string
		candidate      config
		wantDesktop    string
		wantAutologin  string
		wantErrorField string
		wantErrorValue string
	}{
		{name: "linux legacy empty", goos: "linux", candidate: config{}},
		{name: "linux shell true retained", goos: "linux", candidate: config{Shell: true}},
		{name: "linux equal platform", goos: "linux", candidate: config{Platform: "linux"}},
		{name: "linux mismatched platform", goos: "linux", candidate: config{Platform: "windows"}, wantErrorField: "platform", wantErrorValue: "windows"},
		{name: "linux platform case variant", goos: "linux", candidate: config{Platform: "Linux"}, wantErrorField: "platform", wantErrorValue: "Linux"},
		{name: "linux platform whitespace", goos: "linux", candidate: config{Platform: " linux"}, wantErrorField: "platform", wantErrorValue: " linux"},
		{name: "linux desktop rejected", goos: "linux", candidate: config{Desktop: "headless"}, wantErrorField: "desktop", wantErrorValue: "headless"},
		{name: "linux autologin rejected", goos: "linux", candidate: config{Autologin: "none"}, wantErrorField: "autologin", wantErrorValue: "none"},
		{name: "windows legacy empty", goos: "windows", candidate: config{}},
		{name: "windows shell true retained", goos: "windows", candidate: config{Shell: true}},
		{name: "windows equal platform", goos: "windows", candidate: config{Platform: "windows"}},
		{name: "windows mismatched platform", goos: "windows", candidate: config{Platform: "linux"}, wantErrorField: "platform", wantErrorValue: "linux"},
		{name: "windows platform case variant", goos: "windows", candidate: config{Platform: "Windows"}, wantErrorField: "platform", wantErrorValue: "Windows"},
		{name: "windows platform whitespace", goos: "windows", candidate: config{Platform: "windows "}, wantErrorField: "platform", wantErrorValue: "windows "},
		{name: "windows desktop rejected", goos: "windows", candidate: config{Desktop: "xfce"}, wantErrorField: "desktop", wantErrorValue: "xfce"},
		{name: "windows autologin rejected", goos: "windows", candidate: config{Autologin: "console"}, wantErrorField: "autologin", wantErrorValue: "console"},
		{name: "freebsd defaults", goos: "freebsd", candidate: config{}, wantDesktop: "headless", wantAutologin: "none"},
		{name: "freebsd equal platform", goos: "freebsd", candidate: config{Platform: "freebsd"}, wantDesktop: "headless", wantAutologin: "none"},
		{name: "freebsd desktop headless", goos: "freebsd", candidate: config{Desktop: "headless"}, wantDesktop: "headless", wantAutologin: "none"},
		{name: "freebsd desktop xfce", goos: "freebsd", candidate: config{Desktop: "xfce"}, wantDesktop: "xfce", wantAutologin: "none"},
		{name: "freebsd autologin none", goos: "freebsd", candidate: config{Autologin: "none"}, wantDesktop: "headless", wantAutologin: "none"},
		{name: "freebsd autologin console", goos: "freebsd", candidate: config{Autologin: "console"}, wantDesktop: "headless", wantAutologin: "console"},
		{name: "freebsd autologin lightdm deferred pair", goos: "freebsd", candidate: config{Autologin: "lightdm"}, wantDesktop: "headless", wantAutologin: "lightdm"},
		{name: "freebsd shell false", goos: "freebsd", candidate: config{Shell: false}, wantDesktop: "headless", wantAutologin: "none"},
		{name: "freebsd shell true rejected", goos: "freebsd", candidate: config{Shell: true}, wantErrorField: "shell", wantErrorValue: "true"},
		{name: "freebsd mismatched platform", goos: "freebsd", candidate: config{Platform: "linux"}, wantErrorField: "platform", wantErrorValue: "linux"},
		{name: "freebsd platform case variant", goos: "freebsd", candidate: config{Platform: "FreeBSD"}, wantErrorField: "platform", wantErrorValue: "FreeBSD"},
		{name: "freebsd platform whitespace", goos: "freebsd", candidate: config{Platform: "freebsd\t"}, wantErrorField: "platform", wantErrorValue: "freebsd\t"},
		{name: "freebsd invalid desktop", goos: "freebsd", candidate: config{Desktop: "gnome"}, wantErrorField: "desktop", wantErrorValue: "gnome"},
		{name: "freebsd desktop case variant", goos: "freebsd", candidate: config{Desktop: "XFCE"}, wantErrorField: "desktop", wantErrorValue: "XFCE"},
		{name: "freebsd desktop whitespace", goos: "freebsd", candidate: config{Desktop: "xfce "}, wantErrorField: "desktop", wantErrorValue: "xfce "},
		{name: "freebsd invalid autologin", goos: "freebsd", candidate: config{Autologin: "xdm"}, wantErrorField: "autologin", wantErrorValue: "xdm"},
		{name: "freebsd autologin case variant", goos: "freebsd", candidate: config{Autologin: "Console"}, wantErrorField: "autologin", wantErrorValue: "Console"},
		{name: "freebsd autologin whitespace", goos: "freebsd", candidate: config{Autologin: " lightdm"}, wantErrorField: "autologin", wantErrorValue: " lightdm"},
		{name: "unknown GOOS", goos: "plan9", candidate: config{}, wantErrorField: "GOOS", wantErrorValue: "plan9"},
		{name: "linux ignores OS metadata", goos: "linux", candidate: config{OS: "freebsd", Shell: true}},
		{name: "freebsd ignores OS metadata", goos: "freebsd", candidate: config{OS: "linux"}, wantDesktop: "headless", wantAutologin: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: an unvalidated config value and an explicit target GOOS.
			before := tt.candidate

			// When: runtime capabilities are resolved without global state.
			got, err := resolveCapabilities(tt.candidate, tt.goos)

			// Then: the input is immutable and the result or typed error is exact.
			if !reflect.DeepEqual(tt.candidate, before) {
				t.Fatalf("input mutated: got %+v want %+v", tt.candidate, before)
			}
			if tt.wantErrorField != "" {
				var capabilityErr *CapabilityError
				if !errors.As(err, &capabilityErr) {
					t.Fatalf("error = %v, want *CapabilityError", err)
				}
				if capabilityErr.Field != tt.wantErrorField || capabilityErr.Value != tt.wantErrorValue || capabilityErr.GOOS != tt.goos {
					t.Fatalf("error = %+v, want field=%q value=%q GOOS=%q", capabilityErr, tt.wantErrorField, tt.wantErrorValue, tt.goos)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCapabilities() error = %v", err)
			}
			if got.Desktop != tt.wantDesktop || got.Autologin != tt.wantAutologin {
				t.Fatalf("resolved desktop/autologin = %q/%q, want %q/%q", got.Desktop, got.Autologin, tt.wantDesktop, tt.wantAutologin)
			}
			if got.OS != tt.candidate.OS || got.Shell != tt.candidate.Shell || got.Platform != tt.candidate.Platform {
				t.Fatalf("unrelated fields changed: got %+v input %+v", got, tt.candidate)
			}
		})
	}
}
