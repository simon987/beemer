# beemer
![GitHub](https://img.shields.io/github/license/simon987/beemer.svg)
[![Build Status](https://ci.simon987.net/buildStatus/icon?job=beemer_builds)](https://ci.simon987.net/job/beemer_builds/)
[![CodeFactor](https://www.codefactor.io/repository/github/simon987/beemer/badge)](https://www.codefactor.io/repository/github/simon987/beemer)

**beemer** executes a custom command on files written in the watched directory and deletes it.
Optionally, queue files in a .tar file and execute the command when the number of files in the
archive reaches `NUMBER` (see [usage](#usage)).

### Usage

```
NAME:
   beemer - Execute a command on a file after a delay of inactivity

GLOBAL OPTIONS:
  --transfers value, -t value          Number of simultaneous transfers (default: 10)
  --command value, -c value            Will be executed on file write. You can use %file, %name and %dir. Example: "rclone move %file remote:/beem/%dir"
  --wait DELAY, -w DELAY               Files will be beemed after DELAY of inactivity (default: 10s)
  --directory DIRECTORY, -d DIRECTORY  DIRECTORY to watch.
  --tar NUMBER                         Fill a .tar file with up to NUMBER file before executing the beem command.
                                       Set to '1' to disable this feature (default: 1)
  --exclude value, -e value            Exclude files that match the regex pattern
  --help, -h                           show help
  --version, -v                        print the version

```

### Examples

Bundle up to 100 files in a tar file before moving to another directory

\**Note that %dir is always `/tmp/beemer`* when `--tar` is specified

When `--tar NUM` is specified, the beem command will be called at most 
every `NUM` new files.
It will also be called during cleanup when SIGINT (`Ctrl-C`) is received.
```bash
./beemer -w 1s -d ./test --tar 100 -c "mv %file /mnt/store/my_tars/"
```

Upload file to an rclone remote when it has been inactive for at least 30s, 
keeps the directory structure
```bash
./beemer -w 30s -d ./test -c "rclone move %file remote:/beem/%dir"
```

Send file via SSH, ignoring the local directory structure
```bash
./beemer -d ./test -c "scp %file worker@StagingServer:flatdir/"
```

Upload file to transfer.sh, store URLs in `urls.txt`
```bash
./beemer -w 1s -d ./test -c "bash -c \"curl -s -w '\\n' --upload-file %file https://transfer.sh/%name &>> urls.txt\""
```

### Beem command template

| Special sequence | Description | Example |
| :--- | :--- | :--- |
| `%file` | Full path of the modified file | `/tmp/beemer/test/a/myFile.txt` |
| `%name` | Name of the modified file | `myFile.txt` |
| `%dir` | Directory of the modified file, relative to the watched dir. | `test/a` |
