package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestParseConfigCapabilityFailuresPreserveAllState(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "platform mismatch", config: "platform='freebsd'\n"},
		{name: "platform case variant", config: "platform='Linux'\n"},
		{name: "platform whitespace", config: "platform=' linux'\n"},
		{name: "desktop nonempty", config: "desktop='headless'\n"},
		{name: "autologin nonempty", config: "autologin='none'\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldTeamID, oldConf, oldImage, oldConn, oldCount := teamID, conf, image, conn, checkCount
			t.Cleanup(func() {
				teamID, conf, image, conn, checkCount = oldTeamID, oldConf, oldImage, oldConn, oldCount
			})

			// Given: populated config, score, report, connectivity, and progress globals.
			conf = capabilitySentinelConfig()
			image = capabilitySentinelImage()
			conn = &connData{Status: true, OverallColor: "green", OverallStatus: "unchanged", NetColor: "blue", NetStatus: "up", ServerColor: "green", ServerStatus: "up"}
			teamID = "unchanged-team"
			checkCount = 9
			wantConfPointer, wantImagePointer, wantConnPointer := conf, image, conn
			wantConf := cloneCapabilityConfig(conf)
			wantImage := cloneCapabilityImage(image)
			wantConn := *conn

			// When: parsing fails at runtime capability validation.
			err := parseConfig(tt.config)

			// Then: every global value, nested slice, and pointer identity is unchanged.
			if err == nil {
				t.Fatal("parseConfig() error = nil")
			}
			if conf != wantConfPointer || image != wantImagePointer || conn != wantConnPointer {
				t.Fatalf("pointer identity changed: conf=%p image=%p conn=%p", conf, image, conn)
			}
			if !reflect.DeepEqual(*conf, wantConf) || !reflect.DeepEqual(*image, wantImage) || !reflect.DeepEqual(*conn, wantConn) {
				t.Fatalf("state changed: conf=%+v image=%+v conn=%+v", conf, image, conn)
			}
			if teamID != "unchanged-team" || checkCount != 9 {
				t.Fatalf("scalar state changed: teamID=%q checkCount=%d", teamID, checkCount)
			}
		})
	}
}

func capabilitySentinelConfig() *config {
	return &config{Name: "sentinel", Platform: "sentinel-platform", Desktop: "sentinel-desktop", Autologin: "sentinel-autologin", Check: []check{{
		Message:      "unchanged",
		Points:       17,
		Pass:         []cond{{Type: "PathExists", Path: "/sentinel"}},
		Fail:         []cond{{Type: "FileContains", Path: "/sentinel", Value: "value"}},
		PassOverride: []cond{{Type: "CommandContains", Cmd: "true", Value: "value"}},
	}}}
}

func capabilitySentinelImage() *imageData {
	return &imageData{
		Contribs: 1, Detracts: 2, Score: 23, ScoredVulns: 3, TotalPoints: 42,
		Penalties: []scoreItem{{Index: 1, Message: "penalty", Points: -5}},
		Points:    []scoreItem{{Index: 2, Message: "point", Points: 10}},
		Hints:     []hintItem{{Index: 3, Messages: []string{"hint", "nested"}, Points: 4}},
	}
}

func cloneCapabilityConfig(source *config) config {
	cloned := *source
	cloned.Check = slices.Clone(source.Check)
	for index := range cloned.Check {
		cloned.Check[index].Pass = slices.Clone(source.Check[index].Pass)
		cloned.Check[index].Fail = slices.Clone(source.Check[index].Fail)
		cloned.Check[index].PassOverride = slices.Clone(source.Check[index].PassOverride)
	}
	return cloned
}

func cloneCapabilityImage(source *imageData) imageData {
	cloned := *source
	cloned.Penalties = slices.Clone(source.Penalties)
	cloned.Points = slices.Clone(source.Points)
	cloned.Hints = slices.Clone(source.Hints)
	for index := range cloned.Hints {
		cloned.Hints[index].Messages = slices.Clone(source.Hints[index].Messages)
	}
	return cloned
}
