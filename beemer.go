package main

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Beemer struct {
	fileMap       map[string]*File
	tempDir       string
	beemCommand   func(string, string) (string, []string)
	beemChan      chan string
	watcher       *fsnotify.Watcher
	inactiveDelay time.Duration
}

type File struct {
	WaitTimer *time.Timer
	BeemLock  bool
}

func (b Beemer) initWatchDir(watchDir string) {

	logrus.WithField("dir", watchDir).Info("Watching directory for changes")

	err := b.watcher.Add(watchDir)
	_ = filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			err := b.handleDirChange(fsnotify.Event{
				Name: path,
				Op:   fsnotify.Create,
			})

			if err != nil {
				return err
			}
		} else {
			b.handleFileChange(fsnotify.Event{
				Name: path,
				Op:   fsnotify.Create,
			})
		}
		return nil
	})

	if err != nil {
		logrus.Fatal(err)
	}
}

func (b Beemer) getAndResetTimer(name string) *time.Timer {

	file, ok := b.fileMap[name]
	if ok {
		file.WaitTimer.Stop()
		if file.BeemLock == true {
			return nil
		}
	}

	newTimer := time.NewTimer(b.inactiveDelay)
	b.fileMap[name] = &File{
		newTimer,
		false,
	}

	return newTimer
}

func (b Beemer) handleDirChange(event fsnotify.Event) error {

	if event.Op&fsnotify.Create == fsnotify.Create {
		return b.watcher.Add(event.Name)
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		return b.watcher.Remove(event.Name)
	}

	return nil
}

func (b Beemer) handleFileChange(event fsnotify.Event) {

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		t := b.getAndResetTimer(event.Name)
		if t != nil {
			go b.handleFileInactive(t, event.Name)
		}
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		if file, ok := b.fileMap[event.Name]; ok {
			file.WaitTimer.Stop()
			delete(b.fileMap, event.Name)
		}
	}
}

func (b Beemer) handleWatcherEvents() {
	for {
		select {
		case event, ok := <-b.watcher.Events:
			if !ok {
				return
			}

			if isDir(event.Name) {
				err := b.handleDirChange(event)
				if err != nil {
					logrus.Fatal(err)
				}
			} else {
				b.handleFileChange(event)
			}

			if event.Op&fsnotify.Chmod != fsnotify.Chmod {
				logrus.WithFields(logrus.Fields{
					"name": event.Name,
					"op":   event.Op,
				}).Trace("fsnotify")
			}

		case err, ok := <-b.watcher.Errors:
			if !ok {
				return
			}

			logrus.WithError(err).Error("error with watcher")
		}
	}
}

func (b Beemer) work() {
	for {
		select {
		case name := <-b.beemChan:
			b.beemFile(name)
		}
	}
}

func (b Beemer) handleFileInactive(t *time.Timer, name string) {
	<-t.C

	b.fileMap[name].BeemLock = true

	logrus.WithFields(logrus.Fields{
		"name": name,
	}).Infof("has been inactive for %s and will be beemed", b.inactiveDelay)

	b.beemChan <- name
}

func (b Beemer) beemFile(filename string) {

	newName := moveToTempDir(filename, b.tempDir)

	name, args := b.beemCommand(newName, filepath.Dir(filename))

	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logrus.WithField("name", filename).WithError(err).Error(string(out))
	}

	logrus.WithFields(logrus.Fields{
		"name":    newName,
		"command": name,
		"args":    args,
		"out":     string(out),
	}).Trace("Executing beem command")

	err = os.Remove(newName)
	if err != nil && !os.IsNotExist(err) {
		logrus.WithField("name", filename).Error(err)
	}
}
