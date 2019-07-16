package main

import (
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
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
	BeemChan           chan string
}

type File struct {
	WaitTimer *time.Timer
	BeemLock  bool
}

var InactiveDelay time.Duration

var ctx = Ctx{
	FileMap: make(map[string]*File, 0),
}

func main() {
	logrus.SetLevel(logrus.TraceLevel)

	app := cli.NewApp()
	app.Name = "beemer"
	app.Usage = "Execute a command on a file after a delay of inactivity"
	app.Email = "me@simon987.net"
	app.Author = "simon987"
	app.Version = "1.0"

	var cmdString string
	var watchDir string
	var transfers int

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:        "transfers, t",
			Usage:       "Number of simultaneous transfers",
			Destination: &transfers,
			Value:       10,
		},
		cli.StringFlag{
			Name: "command, c",
			Usage: "Will be executed on file write. You can use %file, %name and %dir. " +
				"Example: \"rclone move %file remote:/beem/%dir\"",
			Destination: &cmdString,
		},
		cli.DurationFlag{
			Name:        "wait, w",
			Usage:       "Files will be beemed after `DELAY` of inactivity",
			Destination: &InactiveDelay,
			Value:       time.Second * 10,
		},
		cli.StringFlag{
			Name:        "directory, d",
			Usage:       "`DIRECTORY` to watch.",
			Destination: &watchDir,
		},
	}

	app.Action = func(c *cli.Context) error {

		if !c.IsSet("directory") {
			return errors.New("Directory must be specified")
		}
		ctx.BeemChan = make(chan string, transfers)

		initTempDir()
		ctx.BeemCommand = parseCommand(cmdString)

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logrus.Fatal(err)
		}

		defer watcher.Close()

		go handleWatcherEvents(watcher)

		initWatchDir(watchDir, watcher)

		//TODO gracefully handle SIGINT
		for i := 0; i < transfers; i++ {
			go beemer()
		}

		done := make(chan bool)
		<-done

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(app.OnUsageError)
	}
}

func initWatchDir(watchDir string, watcher *fsnotify.Watcher) {

	logrus.WithField("dir", watchDir).Info("Watching directory for changes")

	err := watcher.Add(watchDir)
	_ = filepath.Walk(watchDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			err := handleDirChange(fsnotify.Event{
				Name: path,
				Op:   fsnotify.Create,
			}, watcher)

			if err != nil {
				return err
			}
		} else {
			handleFileChange(fsnotify.Event{
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

func isDir(name string) bool {
	if stat, err := os.Stat(name); err == nil && stat.IsDir() {
		return true
	}
	return false
}

func handleDirChange(event fsnotify.Event, watcher *fsnotify.Watcher) error {

	if event.Op&fsnotify.Create == fsnotify.Create {
		return watcher.Add(event.Name)
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		return watcher.Remove(event.Name)
	}

	return nil
}

func handleFileChange(event fsnotify.Event) {

	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		t := getAndResetTimer(event.Name)
		if t != nil {
			go handleFileInactive(t, event.Name)
		}
	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		if file, ok := ctx.FileMap[event.Name]; ok {
			file.WaitTimer.Stop()
			delete(ctx.FileMap, event.Name)
		}
	}
}

func handleWatcherEvents(watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if isDir(event.Name) {
				err := handleDirChange(event, watcher)
				if err != nil {
					logrus.Fatal(err)
				}
			} else {
				handleFileChange(event)
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

func beemer() {
	for {
		select {
		case name := <-ctx.BeemChan:
			beemFile(name)
		}
	}
}

func handleFileInactive(t *time.Timer, name string) {
	<-t.C

	ctx.FileMap[name].BeemLock = true

	logrus.WithFields(logrus.Fields{
		"name": name,
	}).Infof("has been inactive for %s and will be beemed", InactiveDelay)

	ctx.BeemChan <- name
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
