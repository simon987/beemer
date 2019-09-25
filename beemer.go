package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"
)

type Beemer struct {
	fileMap        *sync.Map
	tempDir        string
	beemCommand    func(string, string) (string, []string)
	beemChan       chan string
	tarChan        chan string
	watcher        *fsnotify.Watcher
	inactiveDelay  time.Duration
	beemWg         *sync.WaitGroup
	tarWg          *sync.WaitGroup
	tar            *Tar
	tarMaxCount    int
	closing        bool
	excludePattern *regexp.Regexp
	failDir        string
}

type File struct {
	WaitTimer *time.Timer
	BeemLock  bool
}

func (b *Beemer) initWatchDir(watchDir string) {

	logrus.WithField("dir", watchDir).Info("Watching directory for changes")

	err := b.watcher.Add(watchDir)
	_ = filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {

		if b.excludePattern != nil && b.excludePattern.MatchString(path) {
			return nil
		}

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

func (b *Beemer) getAndResetTimer(name string) *time.Timer {

	file, ok := b.fileMap.Load(name)
	if ok {
		file.(*File).WaitTimer.Stop()
		if file.(*File).BeemLock == true {
			return nil
		}
	}

	newTimer := time.NewTimer(b.inactiveDelay)
	b.fileMap.Store(name, &File{
		newTimer,
		false,
	})

	return newTimer
}

func (b *Beemer) handleDirChange(event fsnotify.Event) error {

	if event.Op&fsnotify.Create == fsnotify.Create {
		return b.watcher.Add(event.Name)
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		return b.watcher.Remove(event.Name)
	}

	return nil
}

func (b *Beemer) handleFileChange(event fsnotify.Event) {

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		t := b.getAndResetTimer(event.Name)
		if t != nil {
			go b.handleFileInactive(t, event.Name)
		}
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		if file, ok := b.fileMap.Load(event.Name); ok {
			file.(*File).WaitTimer.Stop()
			b.fileMap.Delete(event.Name)
		}
	}
}

func (b *Beemer) handleWatcherEvents() {
	for {
		select {
		case event, ok := <-b.watcher.Events:
			if !ok {
				return
			}

			if b.excludePattern != nil && b.excludePattern.MatchString(event.Name) {
				continue
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

func (b *Beemer) work() {
	b.beemWg.Add(1)
	for name := range b.beemChan {
		b.beemFile(name)
	}
	b.beemWg.Done()
}

func (b *Beemer) tarWork() {
	b.tarWg.Add(1)
	for filename := range b.tarChan {

		err := b.tar.AddFile(filename)
		if err != nil {
			logrus.WithField("filename", filename).Error(err)
		} else {
			_ = os.Remove(filename)
		}
		logrus.WithFields(logrus.Fields{
			"filename": filename,
			"tar":      b.tar.Name,
			"count":    b.tar.FileCount,
		}).Info("Added file to tar")

		if b.tar.FileCount >= b.tarMaxCount {
			b.beemTar()
		}
	}

	if b.tar.FileCount > 0 {
		logrus.WithField("fileCount", b.tar.FileCount).Info("Beeming partial tar file")
		b.beemTar()
	}

	b.tarWg.Done()
}

func (b *Beemer) beemTar() {

	name := b.tar.Name
	b.tar.Close()
	var err error
	b.tar, err = NewTar(getTarPath(b.tempDir))
	if err != nil {
		logrus.Error(err)
	}

	err = b.executeBeemCommand(name, name)
	if err != nil {
		logrus.WithError(err).Error("Error during beem command! Moved tar file to failDir")
		_ = os.Mkdir(b.failDir, 0700)
		err = moveFile(name, path.Join(b.failDir, path.Base(name)))
		logrus.Info(err)
	}
}

func (b *Beemer) handleFileInactive(t *time.Timer, name string) {
	<-t.C

	file, _ := b.fileMap.Load(name)
	file.(*File).BeemLock = true

	logrus.WithFields(logrus.Fields{
		"name": name,
	}).Infof("has been inactive for %s and will be beemed", b.inactiveDelay)

	if b.closing {
		close(b.beemChan)
		return
	}

	b.beemChan <- name
}

func (b *Beemer) executeBeemCommand(oldName string, newName string) error {

	name, args := b.beemCommand(newName, filepath.Dir(oldName))

	cmd := exec.Command(name, args...)

	// Don't send SIGINT to child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	if cmd.ProcessState.ExitCode() != 0 {
		return errors.New(fmt.Sprintf("Exit code: %d", cmd.ProcessState.ExitCode()))
	}

	logrus.WithFields(logrus.Fields{
		"name":    newName,
		"command": name,
		"args":    args,
		"out":     string(out),
	}).Trace("Executing beem command")

	err = os.Remove(newName)
	if err != nil && !os.IsNotExist(err) {
		logrus.WithField("name", oldName).Error(err)
	}
	return nil
}

func (b *Beemer) beemFile(filename string) {

	newName := moveToTempDir(filename, b.tempDir)

	if b.tar != nil {
		b.tarChan <- newName
	} else {
		err := b.executeBeemCommand(filename, newName)
		if err != nil {
			logrus.WithError(err).Error("Error during beem command, reverting file")
			_ = moveFile(newName, filename)
		}
	}
}
