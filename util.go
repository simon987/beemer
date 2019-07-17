package main

import (
	"github.com/cosiner/argv"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (b Beemer) initTempDir() {
	tmpdir := filepath.Join(os.TempDir(), "work")
	err := os.Mkdir(tmpdir, 0700)
	if err != nil && !os.IsExist(err) {
		logrus.Fatal(err)
	}

	b.tempDir = tmpdir

	logrus.WithField("dir", tmpdir).Infof("Initialized temp dir")
}

func moveToTempDir(name string, tempDir string) string {

	dir := filepath.Join(tempDir, filepath.Dir(name))
	newName := filepath.Join(dir, filepath.Base(name))
	err := os.MkdirAll(dir, 0700)
	if err != nil && !os.IsExist(err) {
		logrus.Fatal(err)
	}

	absName, _ := filepath.Abs(name)
	err = moveFile(absName, newName)
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.WithFields(logrus.Fields{
		"newName": newName,
	}).Trace("Moved to temp dir")

	return newName
}

func moveFile(src string, dst string) error {

	err := os.Rename(src, dst)
	if err != nil {
		err := copyFile(src, dst)
		if err != nil {
			return err
		}

		err = os.Remove(src)
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src string, dst string) error {

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func parseCommand(command string) func(string, string) (string, []string) {
	args, _ := argv.Argv([]rune(command), argv.ParseEnv(os.Environ()), argv.Run)

	logrus.WithField("cmd", args[0]).Info("Parsed beem command")

	return func(name string, dir string) (string, []string) {
		newTokens := make([]string, len(args[0]))
		copy(newTokens, args[0])

		for i := range newTokens {
			newTokens[i] = strings.Replace(newTokens[i], "%file", name, -1)
			newTokens[i] = strings.Replace(newTokens[i], "%dir", dir, -1)
			newTokens[i] = strings.Replace(newTokens[i], "%name", filepath.Base(name), -1)
		}
		return args[0][0], newTokens[1:]
	}
}

func isDir(name string) bool {
	if stat, err := os.Stat(name); err == nil && stat.IsDir() {
		return true
	}
	return false
}

func (b Beemer) dispose() {
	b.watcher.Close()
	err := os.RemoveAll(b.tempDir)
	if err != nil {
		logrus.Fatal(err)
	}
}
