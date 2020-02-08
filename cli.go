package main

import (
	"errors"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

func globalInit() {
	//TODO cmdline flag
	//logrus.SetLevel(logrus.TraceLevel)
}

func main() {

	globalInit()

	app := cli.NewApp()
	app.Name = "work"
	app.Usage = "Execute a command on a file after a delay of inactivity"
	app.Email = "me@simon987.net"
	app.Author = "simon987"
	app.Version = "1.2"

	var cmdString string
	var watchDir string
	var transfers int
	var tarMaxCount int
	var excludePattern string
	var inactiveDelay time.Duration

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
		cli.StringFlag{
			Name:        "exclude, e",
			Usage:       "Exclude files that match this regex pattern",
			Destination: &excludePattern,
		},
		cli.DurationFlag{
			Name:        "wait, w",
			Usage:       "Files will be beemed after `DELAY` of inactivity",
			Destination: &inactiveDelay,
			Value:       time.Second * 10,
		},
		cli.StringFlag{
			Name:        "directory, d",
			Usage:       "`DIRECTORY` to watch.",
			Destination: &watchDir,
		},
		cli.IntFlag{
			Name: "tar",
			Usage: "Fill a .tar file with up to `NUMBER` file before executing the beem command." +
				"Set to '1' to disable this feature",
			Value:       1,
			Destination: &tarMaxCount,
		},
	}

	app.Action = func(c *cli.Context) error {

		if !c.IsSet("directory") {
			return errors.New("directory must be specified")
		}

		beemer := Beemer{
			fileMap:       &sync.Map{},
			beemChan:      make(chan string, transfers),
			tarChan:       make(chan string, 100),
			beemCommand:   parseCommand(watchDir, cmdString),
			inactiveDelay: inactiveDelay,
			beemWg:        &sync.WaitGroup{},
			tarWg:         &sync.WaitGroup{},
			tarMaxCount:   tarMaxCount,
			failDir:       strings.TrimRight(watchDir, "/") + ".fail",
		}

		if excludePattern != "" {
			beemer.excludePattern, _ = regexp.Compile(excludePattern)
			logrus.Infof("Exclude pattern is /%s/", excludePattern)
		}

		beemer.initTempDir()

		beemer.watcher, _ = fsnotify.NewWatcher()

		if tarMaxCount > 1 {
			var err error

			beemer.tar, err = NewTar(getTarPath(beemer.tempDir))
			if err != nil {
				logrus.Fatal(err)
			}
		}

		go beemer.handleWatcherEvents()

		beemer.initWatchDir(watchDir)

		for i := 0; i < transfers; i++ {
			go beemer.work()
		}

		if tarMaxCount > 1 {
			go beemer.tarWork()
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT)

		defer beemer.dispose()

		<-sigChan
		logrus.Info("Received SIGINT")
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(app.OnUsageError)
	}
}
