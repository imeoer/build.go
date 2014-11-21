package main

import (
    "os"
    "os/exec"
    "io/ioutil"
    "encoding/json"
    "bufio"
    "fmt"
    "strings"
    "path/filepath"
    "github.com/go-fsnotify/fsnotify"
)

const (
    CLR_W = ""
    CLR_R = "\x1b[31;1m"
    CLR_G = "\x1b[32;1m"
    CLR_B = "\x1b[34;1m"
)

type ExecMap map[string][]string

// define pattern:(command array) map
var execMap ExecMap

// define command:(process pid) map
var cmdPidMap map[string]int

var watcher *fsnotify.Watcher

// init some global variable
func init() {
    watcher, _ = fsnotify.NewWatcher()
    cmdPidMap = make(map[string]int)
}

func log(color string, info string) {
    fmt.Printf("%s%s%s", color, info + "\n", "\x1b[0m")
}

// watch file change in specified directory
func watch() {
    for path, _ := range execMap {
        dirPath := filepath.Dir(path)
        filepath.Walk(dirPath, func (path string, f os.FileInfo, err error) error {
            if f.IsDir() {
                // defer watcher.Close()
                if err := watcher.Add(path); err != nil {
                    log(CLR_R, err.Error())
                }
            }
            return nil
        })
    }
}

// listen watched file change event
func listen() {
    for {
        select {
            case event := <- watcher.Events:
                handle(event)
            case err := <- watcher.Errors:
                log(CLR_R, err.Error())
        }
    }
}

func handle(event fsnotify.Event) {
    // get change file info
    fileName := event.Name
    // if changed file path match define in execMap, run command
    for pattern, cmdAry := range execMap {
        if ok, err := filepath.Match(pattern, fileName); err == nil && ok {
            // exec command by array order
            for _, cmd := range cmdAry {
                run(cmd)
            }
        }
    }
}

func run(command string) {
    log(CLR_W, command)
    // kill last process of the same name
    if lastPid, ok := cmdPidMap[command]; ok {
        if process, err := os.FindProcess(lastPid); err == nil {
            process.Kill()
        }
    }
    // parse command line auguments
    cmdAry := strings.Split(command, " ")
    // prepare exec command
    cmd := exec.Command(cmdAry[0], cmdAry[1:]...)
    // start print stdout and stderr of process
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    out := bufio.NewScanner(stdout)
    err := bufio.NewScanner(stderr)
    // print stdout
    go func() {
        for out.Scan() {
            log(CLR_W, out.Text())
        }
    }()
    // print stdin
    go func() {
        for err.Scan() {
            log(CLR_R, err.Text())
        }
    }()
    // exec command
    cmd.Start()
    // cache command process pid
    pid := cmd.Process.Pid
    cmdPidMap[command] = pid
}

func main() {
    // parse json config file, get execMap
    file, err := ioutil.ReadFile("./build.json")
    if err != nil {
        log(CLR_R, err.Error())
        os.Exit(1)
    }
    if err := json.Unmarshal(file, &execMap); err != nil {
        log(CLR_R, err.Error())
        os.Exit(1)
    }
    // use for always running
    done := make(chan bool)
    // start to watch file change
    watch()
    // listen watching file change
    go listen()
    <- done
}
