package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	alpm "github.com/jguer/go-alpm"
)

func setPaths() error {
	if _configHome, set := os.LookupEnv("XDG_CONFIG_HOME"); set {
		if _configHome == "" {
			return fmt.Errorf("XDG_CONFIG_HOME set but empty")
		}
		configHome = filepath.Join(_configHome, "yay")
	} else if _configHome, set := os.LookupEnv("HOME"); set {
		if _configHome == "" {
			return fmt.Errorf("HOME set but empty")
		}
		configHome = filepath.Join(_configHome, ".config/yay")
	} else {
		return fmt.Errorf("XDG_CONFIG_HOME and HOME unset")
	}

	if _cacheHome, set := os.LookupEnv("XDG_CACHE_HOME"); set {
		if _cacheHome == "" {
			return fmt.Errorf("XDG_CACHE_HOME set but empty")
		}
		cacheHome = filepath.Join(_cacheHome, "yay")
	} else if _cacheHome, set := os.LookupEnv("HOME"); set {
		if _cacheHome == "" {
			return fmt.Errorf("XDG_CACHE_HOME set but empty")
		}
		cacheHome = filepath.Join(_cacheHome, ".cache/yay")
	} else {
		return fmt.Errorf("XDG_CACHE_HOME and HOME unset")
	}

	configFile = filepath.Join(configHome, configFileName)
	vcsFile = filepath.Join(cacheHome, vcsFileName)

	return nil
}

func initConfig() error {
	defaultSettings(&config)

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(configFile), 0755)
		if err != nil {
			err = fmt.Errorf("Unable to create config directory:\n%s\n"+
				"The error was:\n%s", filepath.Dir(configFile), err)
			return err
		}
		// Save the default config if nothing is found
		config.saveConfig()
		return err
	}

	cfile, err := os.OpenFile(configFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Printf("Error reading config: %s\n", err)
		return err
	}
	defer cfile.Close()

	decoder := json.NewDecoder(cfile)
	if err := decoder.Decode(&config); err != nil {
		fmt.Println("Loading default Settings.\nError reading config:", err)
		defaultSettings(&config)
		return err
	}

	if _, err := os.Stat(config.BuildDir); os.IsNotExist(err) {
		if err = os.MkdirAll(config.BuildDir, 0755); err != nil {
			return fmt.Errorf("Unable to create BuildDir directory:\n%s\n"+
				"The error was:\n%s", config.BuildDir, err)
		}
	}

	return nil
}

func initVCS() (err error) {
	if _, err = os.Stat(vcsFile); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(vcsFile), 0755)
		if err != nil {
			err = fmt.Errorf("Unable to create vcs directory:\n%s\n"+
				"The error was:\n%s", filepath.Dir(configFile), err)
			return
		}
		return
	}
	vfile, err := os.OpenFile(vcsFile, os.O_RDONLY|os.O_CREATE, 0644)
	if err == nil {
		defer vfile.Close()
		decoder := json.NewDecoder(vfile)
		_ = decoder.Decode(&savedInfo)
	}
	return err
}

func initAlpm() (err error) {
	var value string
	var exists bool

	alpmConf, err = readAlpmConfig(config.PacmanConf)
	if err != nil {
		err = fmt.Errorf("Unable to read Pacman conf: %s", err)
		return
	}

	value, _, exists = cmdArgs.getArg("dbpath", "b")
	if exists {
		alpmConf.DBPath = value
	}

	value, _, exists = cmdArgs.getArg("root", "r")
	if exists {
		alpmConf.RootDir = value
	}

	value, _, exists = cmdArgs.getArg("arch")
	if exists {
		alpmConf.Architecture = value
	}

	value, _, exists = cmdArgs.getArg("ignore")
	if exists {
		alpmConf.IgnorePkg = append(alpmConf.IgnorePkg, strings.Split(value, ",")...)
	}

	value, _, exists = cmdArgs.getArg("ignoregroup")
	if exists {
		alpmConf.IgnoreGroup = append(alpmConf.IgnoreGroup, strings.Split(value, ",")...)
	}

	//TODO
	//current system does not allow duplicate arguments
	//but pacman allows multiple cachdirs to be passed
	//for now only handle one cache dir
	value, _, exists = cmdArgs.getArg("cachdir")
	if exists {
		alpmConf.CacheDir = []string{value}
	}

	value, _, exists = cmdArgs.getArg("gpgdir")
	if exists {
		alpmConf.GPGDir = value
	}

	err = initAlpmHandle()
	if err != nil {
		return
	}

	value, _, _ = cmdArgs.getArg("color")
	if value == "always" || value == "auto" {
		useColor = true
	} else if value == "never" {
		useColor = false
	} else {
		useColor = alpmConf.Options&alpm.ConfColor > 0
	}

	return
}

func initAlpmHandle() error {
	if alpmHandle != nil {
		if err := alpmHandle.Release(); err != nil {
			return err
		}
	}
	var err error
	if alpmHandle, err = alpmConf.CreateHandle(); err != nil {
		return fmt.Errorf("Unable to CreateHandle: %s", err)
	}

	alpmHandle.SetQuestionCallback(questionCallback)
	alpmHandle.SetLogCallback(logCallback)
	return nil
}

// cleanupAndExit is responsible for cleaning up any handlers and also for
// ending the program with os.Exit, using given exit code.
// Passing in a slice of errors is optional.
func cleanupAndExit(errs ...error) {
	if alpmHandle != nil {
		if err := alpmHandle.Release(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// simpleFn represents a simple function that takes no arguments and returns an error
type simpleFn func() error

// runTheseOrExit runs the given functions.
// If one of them fails, cleanupAndExit is called.
func runTheseOrExit(fns ...simpleFn) {
	for _, fn := range fns {
		if err := fn(); err != nil {
			cleanupAndExit(err)
		}
	}
}

func main() {
	if os.Geteuid() == 0 {
		fmt.Println("Please avoid running yay as root/sudo.")
	}
	runTheseOrExit([]simpleFn{cmdArgs.parseCommandLine, setPaths, initConfig}...)
	cmdArgs.extractYayOptions()
	runTheseOrExit([]simpleFn{initVCS, initAlpm, handleCmd}...)
	cleanupAndExit()
}
