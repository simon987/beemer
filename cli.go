package main

import (
	"errors"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func globalInit() {
	//TODO cmdline flag
	logrus.SetLevel(logrus.TraceLevel)
}

func main() {

	globalInit()

	app := cli.NewApp()
	app.Name = "work"
	app.Usage = "Execute a command on a file after a delay of inactivity"
	app.Email = "me@simon987.net"
	app.Author = "simon987"
	app.Version = "1.1"

	var cmdString string
	var watchDir string
	var transfers int
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
	}

	app.Action = func(c *cli.Context) error {

		if !c.IsSet("directory") {
			return errors.New("directory must be specified")
		}

		beemer := Beemer{
			fileMap:       make(map[string]*File, 0),
			beemChan:      make(chan string, transfers),
			beemCommand:   parseCommand(cmdString),
			inactiveDelay: inactiveDelay,
			globalWg:      &sync.WaitGroup{},
		}

		beemer.initTempDir()

		beemer.watcher, _ = fsnotify.NewWatcher()

		go beemer.handleWatcherEvents()

		beemer.initWatchDir(watchDir)

		for i := 0; i < transfers; i++ {
			go beemer.work()
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
