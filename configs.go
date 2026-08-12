package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// parseConfig takes the config content as a string and attempts to parse it
// into the conf struct based on the TOML spec.
func parseConfig(configContent string) error {
	return parseConfigWithCapabilities(configContent, runtime.GOOS, resolveRuntimeCapabilities)
}

func parseConfigWithCapabilities(configContent, goos string, resolve func(config) (config, error)) error {
	if configContent == "" {
		return fmt.Errorf("configuration is empty")
	}
	candidate := &config{}
	md, err := toml.Decode(configContent, candidate)
	if err != nil {
		return fmt.Errorf("decode TOML for GOOS %s: %w", goos, err)
	}

	// If there's no remote, local must be enabled.
	if candidate.Remote == "" {
		candidate.Local = true
		if candidate.DisableRemoteEncryption {
			return fmt.Errorf("remote encryption cannot be disabled if remote is not enabled")
		}
	} else {
		if candidate.Remote[len(candidate.Remote)-1] == '/' {
			return fmt.Errorf("remote URL must not end with a slash: try %s", candidate.Remote[:len(candidate.Remote)-1])
		}
		if candidate.Name == "" {
			return fmt.Errorf("image name is required when remote is enabled")
		}
		if candidate.Password == "" && !candidate.DisableRemoteEncryption {
			return fmt.Errorf("password is required when remote is enabled")
		}
		if candidate.DisableRemoteEncryption && candidate.Password != "" {
			warn("Remote encryption is disabled, but a password is still defined!")
		}
	}
	resolved, err := resolve(*candidate)
	if err != nil {
		return err
	}
	candidate = &resolved

	if err := validateConfigConditions(candidate, goos); err != nil {
		return err
	}
	if verboseEnabled {
		for _, undecoded := range md.Undecoded() {
			if len(undecoded) == 3 && strings.EqualFold(undecoded[0], "check") {
				switch strings.ToLower(undecoded[1]) {
				case "pass", "fail", "passoverride":
					continue
				}
			}
			warn("Undecoded scoring configuration key \"" + undecoded.String() + "\" will not be used.")
		}
	}

	// Check if the config version matches ours.
	if candidate.Version != version {
		warn("Scoring version does not match Aeacus version! Compatibility issues may occur.")
		info("Consider updating your config to include:")
		info("    version = '" + version + "'")
	}

	for i, check := range candidate.Check {
		if len(check.Pass) == 0 && len(check.PassOverride) == 0 {
			warn("Check " + fmt.Sprintf("%d", i+1) + " does not define any possible ways to pass!")
		}
	}
	conf = candidate
	return nil
}

// writeConfig writes the in-memory config to disk as the an encrypted
// configuration file.
func writeConfig() error {
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(conf); err != nil {
		return fmt.Errorf("encode scoring configuration: %w", err)
	}

	dataPath := dirPath + scoringData
	encryptedConfig, err := encryptConfig(buf.String())
	if err != nil {
		return fmt.Errorf("encrypt scoring configuration: %w", err)
	} else if verboseEnabled {
		info("Writing data to " + dataPath + "...")
	}

	if err := os.WriteFile(dataPath, []byte(encryptedConfig), 0o644); err != nil {
		return fmt.Errorf("persist encrypted scoring configuration: %w", err)
	}
	return nil
}

// ReadConfig parses the scoring configuration file.
func readConfig() error {
	fileContent, err := readFile(dirPath + scoringConf)
	if err != nil {
		return fmt.Errorf("configuration file (%s%s) not found: %w", dirPath, scoringConf, err)
	}
	if err := parseConfig(fileContent); err != nil {
		return err
	}
	assignPoints()
	assignDescriptions()
	if verboseEnabled {
		printConfig()
	}
	obfuscateConfig()
	return nil
}

// PrintConfig offers a printed representation of the config, as parsed
// by readData and parseConfig.
func printConfig() {
	pass("Configuration " + dirPath + scoringConf + " validity check passed!")
	blue("CONF", scoringConf)
	if conf.Version != "" {
		pass("Version:", conf.Version)
	}
	if conf.Title == "" {
		red("MISS", "Title:", "N/A")
	} else {
		pass("Title:", conf.Title)
	}
	if conf.Name == "" {
		red("MISS", "Name:", "N/A")
	} else {
		pass("Name:", conf.Name)
	}
	if conf.OS == "" {
		red("MISS", "OS:", "N/A")
	} else {
		pass("OS:", conf.OS)
	}
	if conf.User == "" {
		red("MISS", "User:", "N/A")
	} else {
		pass("User:", conf.User)
	}
	if conf.Remote != "" {
		pass("Remote:", conf.Remote)
	}
	if conf.DisableRemoteEncryption {
		pass("Remote Encryption:", "Disabled")
	} else {
		pass("Remote Encryption:", "Enabled")
	}
	if conf.Local {
		pass("Local:", conf.Local)
	}
	if conf.EndDate != "" {
		pass("End Date:", conf.EndDate)
	}
	for i, check := range conf.Check {
		green("CHCK", fmt.Sprintf("Check %d (%d points):", i+1, check.Points))
		fmt.Println("Message:", check.Message)
		if check.Category != "" {
			fmt.Println("Category:", check.Category)
		}
		for _, c := range check.Pass {
			fmt.Println("Pass Condition:")
			fmt.Print(c)
		}
		for _, c := range check.PassOverride {
			fmt.Println("PassOverride Condition:")
			fmt.Print(c)
		}
		for _, c := range check.Fail {
			fmt.Println("Fail Condition:")
			fmt.Print(c)
		}
	}
}

func obfuscateConfig() {
	if debugEnabled {
		debug("Obfuscating configuration...")
	}
	if err := obfuscateData(&conf.Password); err != nil {
		fail(err.Error())
	}
	for i, check := range conf.Check {
		if err := obfuscateData(&conf.Check[i].Message); err != nil {
			fail(err.Error())
		}
		if conf.Check[i].Hint != "" {
			if err := obfuscateData(&conf.Check[i].Hint); err != nil {
				fail(err.Error())
			}
		}
		for j := range check.Pass {
			if err := obfuscateCond(&conf.Check[i].Pass[j]); err != nil {
				fail(err.Error())
			}
		}
		for j := range check.PassOverride {
			if err := obfuscateCond(&conf.Check[i].PassOverride[j]); err != nil {
				fail(err.Error())
			}
		}
		for j := range check.Fail {
			if err := obfuscateCond(&conf.Check[i].Fail[j]); err != nil {
				fail(err.Error())
			}
		}
	}
}

// obfuscateCond is a convenience function to obfuscate all string fields of a
// struct using reflection. It assumes all struct fields are strings.
func obfuscateCond(c *cond) error {
	s := reflect.ValueOf(c).Elem()
	for i := 0; i < s.NumField(); i++ {
		if s.Field(i).Kind() != reflect.String {
			continue
		}
		datum := s.Field(i).String()
		if err := obfuscateData(&datum); err != nil {
			return err
		}
		s.Field(i).SetString(datum)
	}
	return nil
}

// deobfuscateCond is a convenience function to deobfuscate all string fields
// of a struct using reflection.
func deobfuscateCond(c *cond) error {
	s := reflect.ValueOf(c).Elem()
	for i := 0; i < s.NumField(); i++ {
		if s.Field(i).Kind() != reflect.String {
			continue
		}
		datum := s.Field(i).String()
		if err := deobfuscateData(&datum); err != nil {
			return err
		}
		s.Field(i).SetString(datum)
	}
	return nil
}

func xor(key, plaintext string) string {
	ciphertext := make([]byte, len(plaintext))
	for i := 0; i < len(plaintext); i++ {
		ciphertext[i] = key[i%len(key)] ^ plaintext[i]
	}
	return string(ciphertext)
}

func hexEncode(inputString string) string {
	return hex.EncodeToString([]byte(inputString))
}

func hexDecode(inputString string) (string, error) {
	result, err := hex.DecodeString(inputString)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
