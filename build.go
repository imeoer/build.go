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
    "path/filepath"
    "gopkg.in/yaml.v2"
    "github.com/codegangsta/cli"
    "github.com/go-fsnotify/fsnotify"
)

// Color define for log
const (
    CLR_W = ""
    CLR_R = "\x1b[31;1m"
    CLR_G = "\x1b[32;1m"
    CLR_B = "\x1b[34;1m"
)

// Build define by parse config json
type BuildMap struct {
    Variable map[string]string
    Task map[string][]string
    Watch map[string]string
}

// Storaged data form json config
var buildMap BuildMap

// Variable(${}) match regex
var varRegex *regexp.Regexp

// Global watcher for file change
var watcher *fsnotify.Watcher

// Watch dir path map, keep unique
var watchDir map[string]bool

// Need print detail log flag
var needDetailLog bool

// Print colorful log
func log(color string, info string) {
    if color == CLR_G && !needDetailLog {
        return
    }
    fmt.Printf("%s%s%s", color, info + "\n", "\x1b[0m")
}

// Watch file change in specified directory
func startWatch() {
    log(CLR_G, "RUN: watching file change...")
    for path, _ := range buildMap.Watch {
        path = parseVariable(path)
        if matchPath, err := filepath.Glob(path); err == nil {
            for _, path := range matchPath {
                dirPath := filepath.Dir(path)
                if _, ok := watchDir[dirPath]; !ok {
                    if err := watcher.Add(dirPath); err != nil {
                        log(CLR_R, err.Error())
                    }
                    watchDir[dirPath] = true
                }
            }
        } else {
            log(CLR_R, err.Error())
            os.Exit(1)
        }
    }
    // Listen watched file change event
    go func() {
        for {
            select {
                case event := <- watcher.Events:
                    if event.Op == fsnotify.Write {
                        // Handle when file change
                        handleWatch(event)
                    }
                case err := <- watcher.Errors:
                    log(CLR_R, err.Error())
            }
        }
    }()
}

// When file change, run task to handle
func handleWatch(event fsnotify.Event) {
    // Get change file info
    fileName := event.Name
    // If changed file path match define in build map, run task
    for pattern, task := range buildMap.Watch {
        pattern = parseVariable(pattern)
        if ok, err := filepath.Match(pattern, fileName); err == nil && ok {
            // Exec task by task name
            if taskName := extractRef(task); taskName != "" {
                go runTask(taskName, false)
            }
        }
    }
}

// Replace ${} refrence to real value
func parseVariable(str string) string {
    refAry := varRegex.FindAllString(str, -1)
    if len(refAry) > 0 {
        for _, ref := range refAry {
            varName := extractRef(ref)
            if varValue, ok := buildMap.Variable[varName]; ok {
                str = strings.Replace(str, ref, varValue, 1)
            } else {
                log(CLR_R, "ERR: Variable \"" + varName + "\" Not Found")
                os.Exit(1)
            }
        }
    }
    return str
}

// Extract ${} refrence
func extractRef(str string) string {
    if len(str) > 3 && str[0:2] == "${" && string(str[len(str) - 1]) == "}" {
        str = strings.Replace(str, "${", "", -1)
        str = strings.Replace(str, "}", "", -1)
        return str
    }
    return ""
}

// Run task defined in build map
func runTask(task string, forceDaemon bool) {
    // If task has # prefix, run in non-block mode
    daemon := false
    if string(task[0]) == "#" {
        daemon = true
        task = task[1:]
    } else if forceDaemon {
        daemon = true
    }
    if cmdAry, ok := buildMap.Task[task]; ok {
        // Exec command by array order
        for idx, cmd := range cmdAry {
            err := runCMD(cmd, daemon)
            taskName := task + " [" + strconv.Itoa(idx) + "]"
            log(CLR_G, "RUN: " + taskName)
            if err != nil {
                log(CLR_G, "RUN TERMINATED: " + taskName)
                break
            }
        }
    } else {
        log(CLR_R, "ERR: Task \"" + task + "\" Not Found")
        os.Exit(1)
    }
}

// Run command defined in task
func runCMD(command string, daemon bool) error {
    // Run task if command is task name
    if taskName := extractRef(command); taskName != "" {
        runTask(taskName, daemon)
        return nil
    }
    // Parse variable in command
    command = parseVariable(command)
    // Prepare exec command
    cmd := exec.Command("/bin/sh", "-c", command)
    // Start print stdout and stderr of process
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    out := bufio.NewScanner(stdout)
    err := bufio.NewScanner(stderr)
    // Print stdout
    go func() {
        for out.Scan() {
            log(CLR_W, out.Text())
        }
    }()
    // Print stdin
    go func() {
        for err.Scan() {
            log(CLR_R, err.Text())
        }
    }()
    // Exec command
    if daemon {
        // Run in non-block mode
        go cmd.Run()
        return nil
    }
    return cmd.Run()
}

// Init some global variable
func init() {
    watcher, _ = fsnotify.NewWatcher()
    varRegex = regexp.MustCompile("\\${[A-Za-z0-9_-]+}")
    watchDir = make(map[string]bool)
}

func main() {
    // Init cli app
    app := cli.NewApp()
    app.Name = "Build.go"
    app.Usage = "A Simple Automation Task Build Tool"
    app.Author = "https://github.com/imeoer"
    app.Email = "imeoer@gmail.com"
    app.Version = "0.1.0"
    app.Flags = []cli.Flag {
        cli.StringFlag{
            Name: "config, c",
            Value: "build.yml",
            Usage: "Build.go YAML Format Config File",
        },
        cli.BoolFlag{
            Name: "detail, d",
            Usage: "Show detail log when running build",
        },
    }
    app.Action = func(c *cli.Context) {
        // Get config file and task name from command line
        var taskName, configFile string
        if len(c.Args()) > 0 {
            taskName = c.Args()[0]
        } else {
            taskName = "default"
        }
        configFile = c.String("config")
        needDetailLog = c.Bool("detail")
        // Parse json config file, get build map
        file, err := ioutil.ReadFile(configFile)
        if err != nil {
            log(CLR_R, err.Error())
            os.Exit(1)
        }
        if err := yaml.Unmarshal(file, &buildMap); err != nil {
            log(CLR_R, "Config: " + err.Error())
            os.Exit(1)
        }
        // Prehandle for config file
        // Support nest variable
        for name, value := range buildMap.Variable {
            buildMap.Variable[name] = parseVariable(value)
        }
        // Use for always running
        done := make(chan bool)
        // Start to watch file change
        startWatch()
        // Run specified task, if not specified, run default task
        runTask(taskName, false)
        // Keep watch if has watch config
        if len(buildMap.Watch) != 0 {
            <- done
        }
    }
    app.Run(os.Args)
}
