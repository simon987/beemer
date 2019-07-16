# beemer
![GitHub](https://img.shields.io/github/license/simon987/beemer.svg)
[![Build Status](https://ci.simon987.net/buildStatus/icon?job=beemer_builds)](https://ci.simon987.net/job/beemer_builds/)
[![CodeFactor](https://www.codefactor.io/repository/github/simon987/beemer/badge)](https://www.codefactor.io/repository/github/simon987/beemer)

**beemer** executes a custom command on files written in the watched directory and deletes it.

### Usage

```
NAME:
   beemer - Execute a command on a file after a delay of inactivity

GLOBAL OPTIONS:
  --transfers value, -t value          Number of simultaneous transfers (default: 10)
  --command value, -c value            Will be executed on file write. You can use %file, %name and %dir. Example: "rclone move %file remote:/beem/%dir"
  --wait DELAY, -w DELAY               Files will be beemed after DELAY of inactivity (default: 10s)
  --directory DIRECTORY, -d DIRECTORY  DIRECTORY to watch.
  --help, -h                           show help
  --version, -v                        print the version

```

### Examples

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
