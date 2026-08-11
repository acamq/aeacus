package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestReadConfigValidationFailurePreservesAllState(t *testing.T) {
	oldDir, oldTeamID, oldConf, oldImage, oldConn, oldCount := dirPath, teamID, conf, image, conn, checkCount
	t.Cleanup(func() {
		dirPath, teamID, conf, image, conn, checkCount = oldDir, oldTeamID, oldConf, oldImage, oldConn, oldCount
	})

	// Given: populated config, score, report, connectivity, and progress globals.
	conf = &config{Name: "sentinel", Check: []check{{
		Message:      "unchanged",
		Points:       17,
		Pass:         []cond{{Type: "PathExists", Path: "/sentinel"}},
		Fail:         []cond{{Type: "FileContains", Path: "/sentinel", Value: "value"}},
		PassOverride: []cond{{Type: "CommandContains", Cmd: "true", Value: "value"}},
	}}}
	image = &imageData{
		Contribs: 1, Detracts: 2, Score: 23, ScoredVulns: 3, TotalPoints: 42,
		Penalties: []scoreItem{{Index: 1, Message: "penalty", Points: -5}},
		Points:    []scoreItem{{Index: 2, Message: "point", Points: 10}},
		Hints:     []hintItem{{Index: 3, Messages: []string{"hint", "nested"}, Points: 4}},
	}
	conn = &connData{Status: true, OverallColor: "green", OverallStatus: "unchanged", NetColor: "blue", NetStatus: "up", ServerColor: "green", ServerStatus: "up"}
	teamID = "unchanged-team"
	checkCount = 9
	wantConfPointer, wantImagePointer, wantConnPointer := conf, image, conn
	wantConf := *conf
	wantConf.Check = slices.Clone(conf.Check)
	for index := range wantConf.Check {
		wantConf.Check[index].Pass = slices.Clone(conf.Check[index].Pass)
		wantConf.Check[index].Fail = slices.Clone(conf.Check[index].Fail)
		wantConf.Check[index].PassOverride = slices.Clone(conf.Check[index].PassOverride)
	}
	wantImage := *image
	wantImage.Penalties = slices.Clone(image.Penalties)
	wantImage.Points = slices.Clone(image.Points)
	wantImage.Hints = slices.Clone(image.Hints)
	for index := range wantImage.Hints {
		wantImage.Hints[index].Messages = slices.Clone(image.Hints[index].Messages)
	}
	wantConn := *conn
	tempDir := t.TempDir()
	dirPath = tempDir + string(os.PathSeparator)
	invalid := "[[check]]\npoints=50\n[[check.pass]]\ntype='PathExists'\n"
	if err := os.WriteFile(filepath.Join(tempDir, scoringConf), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}

	// When: loading fails semantic validation before scoring setup.
	err := readConfig()

	// Then: every touched global, nested slice, and pointer identity is unchanged.
	if err == nil {
		t.Fatal("readConfig() error = nil")
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
}
