package main

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Ctx struct {
	GlobalUploadTicker time.Ticker
	FileMap            map[string]*File
	TempDir            string
	BeemCommand        func(string, string) (string, []string)
}

type File struct {
	WaitTimer *time.Timer
	BeemLock  bool
}

var CmdString = "rclone move %file remote:/beem/%dir"
var InactiveDelay = time.Second * 5

var ctx = Ctx{
	FileMap: make(map[string]*File, 0),
}

func main() {
	logrus.SetLevel(logrus.TraceLevel)

	initTempDir()
	ctx.BeemCommand = parseCommand(CmdString)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Fatal(err)
	}

	defer watcher.Close()

	go handleFileChange(watcher)

	err = watcher.Add("./test")
	if err != nil {
		logrus.Fatal(err)
	}

	//TODO gracefully handle SIGINT
	done := make(chan bool)
	<-done
}

func getAndResetTimer(name string) *time.Timer {

	file, ok := ctx.FileMap[name]
	if ok {
		file.WaitTimer.Stop()
		if file.BeemLock == true {
			return nil
		}
	}

	newTimer := time.NewTimer(InactiveDelay)
	ctx.FileMap[name] = &File{
		newTimer,
		false,
	}

	return newTimer
}

func handleFileChange(watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:

			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if stat, err := os.Stat(event.Name); err == nil && stat.IsDir() {
					logrus.WithField("name", event.Name).Info("Created dir")
					err = watcher.Add(event.Name)
					if err != nil {
						logrus.Fatal(err)
					}
				} else {
					t := getAndResetTimer(event.Name)
					if t != nil {
						go handleFileInactive(t, event.Name)
					}
				}
			} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				if stat, err := os.Stat(event.Name); err == nil && stat.IsDir() {
					logrus.WithField("name", event.Name).Info("Removed dir")
					err = watcher.Remove(event.Name)
					if err != nil {
						logrus.Fatal(err)
					}
				} else if file, ok := ctx.FileMap[event.Name]; ok {
					file.WaitTimer.Stop()
					delete(ctx.FileMap, event.Name)
				}
			}

			if event.Op&fsnotify.Chmod != fsnotify.Chmod {
				logrus.WithFields(logrus.Fields{
					"name": event.Name,
					"op":   event.Op,
				}).Trace("fsnotify")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logrus.WithError(err).Error("error with Watcher")
		}
	}
}

func handleFileInactive(t *time.Timer, name string) {
	<-t.C

	ctx.FileMap[name].BeemLock = true

	logrus.WithFields(logrus.Fields{
		"name": name,
	}).Infof("has been inactive for %s and will be beemed", InactiveDelay)

	beemFile(name)
}

func beemFile(filename string) {

	newName := moveToTempDir(filename)

	name, args := ctx.BeemCommand(newName, filepath.Dir(filename))

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
