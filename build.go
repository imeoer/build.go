package main

import (
    "os"
    "fmt"
    "bufio"
    "regexp"
    "strconv"
    "strings"
    "os/exec"
    "io/ioutil"
    "encoding/json"
    "path/filepath"
    "github.com/go-fsnotify/fsnotify"
)

// color define for log
const (
    CLR_W = ""
    CLR_R = "\x1b[31;1m"
    CLR_G = "\x1b[32;1m"
    CLR_B = "\x1b[34;1m"
)

// build define by parse config json
type BuildMap struct {
    Variable map[string]string `json:"variable"`
    Task map[string][]string `json:"task"`
    Watch map[string]string `json:"watch"`
}

// storaged data form json config
var buildMap BuildMap

// variable(${}) match regex
var varRegex *regexp.Regexp

// global watcher for file change
var watcher *fsnotify.Watcher

// print colorful log
func log(color string, info string) {
    fmt.Printf("%s%s%s", color, info + "\n", "\x1b[0m")
}

// watch file change in specified directory
func watch() {
    for path, _ := range buildMap.Watch {
        path = parseVariable(path)
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
                if event.Op == fsnotify.Write {
                    // handle when file change
                    handle(event)
                }
            case err := <- watcher.Errors:
                log(CLR_R, err.Error())
        }
    }
}

// when file change, run task to handle
func handle(event fsnotify.Event) {
    // get change file info
    fileName := event.Name
    // if changed file path match define in build map, run task
    for pattern, task := range buildMap.Watch {
        pattern = parseVariable(pattern)
        if ok, err := filepath.Match(pattern, fileName); err == nil && ok {
            // exec task by task name
            if taskName := extractRef(task); taskName != "" {
                go runTask(taskName, false)
            }
        }
    }
}

// replace ${} refrence to real value
func parseVariable(str string) string {
    refAry := varRegex.FindAllString(str, -1)
    if len(refAry) > 0 {
        for _, ref := range refAry {
            varName := extractRef(ref)
            if varValue, ok := buildMap.Variable[varName]; ok {
                str = strings.Replace(str, ref, varValue, 1)
            }
        }
    }
    return str
}

// extract ${} refrence
func extractRef(str string) string {
    if len(str) > 3 && str[0:2] == "${" && string(str[len(str) - 1]) == "}" {
        str = strings.Replace(str, "${", "", -1)
        str = strings.Replace(str, "}", "", -1)
        return str
    }
    return ""
}

// run task defined in build map
func runTask(task string, forceDaemon bool) {
    // if task has # prefix, run in non-block mode
    daemon := false
    if string(task[0]) == "#" {
        daemon = true
        task = task[1:]
    } else if forceDaemon {
        daemon = true
    }
    if cmdAry, ok := buildMap.Task[task]; ok {
        // exec command by array order
        for idx, cmd := range cmdAry {
            err := runCMD(cmd, daemon)
            log(CLR_G, "RUN: " + task + " [" + strconv.Itoa(idx) + "]")
            if err != nil {
                log(CLR_R, err.Error())
                continue
            }
        }
    } else {
        log(CLR_R, "ERR: " + task + " Not Found")
        os.Exit(1)
    }
}

// run command defined in task
func runCMD(command string, daemon bool) error {
    // run task if command is task name
    if taskName := extractRef(command); taskName != "" {
        runTask(taskName, daemon)
        return nil
    }
    // parse variable in command
    command = parseVariable(command)
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
    if daemon {
        // run in non-block mode
        go cmd.Run()
        return nil
    }
    return cmd.Run()
}

// init some global variable
func init() {
    watcher, _ = fsnotify.NewWatcher()
    varRegex = regexp.MustCompile("\\${[A-Za-z0-9_-]+}")
}

func main() {
    // parse json config file, get build map
    file, err := ioutil.ReadFile("./build.json")
    if err != nil {
        log(CLR_R, err.Error())
        os.Exit(1)
    }
    if err := json.Unmarshal(file, &buildMap); err != nil {
        log(CLR_R, "Config Parse: " + err.Error())
        os.Exit(1)
    }
    // use for always running
    done := make(chan bool)
    // start to watch file change
    watch()
    // listen watching file change
    go listen()
    // run specified task, if not specified, run default task
    if len(os.Args) > 1 {
        runTask(os.Args[1], false)
    } else {
        runTask("default", false)
    }
    <- done
}
