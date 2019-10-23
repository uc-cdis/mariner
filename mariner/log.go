package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	cwl "github.com/uc-cdis/cwl.go"
)

// TODO - write json encodings for all this AFTER implementing it
// so I don't constantly add/delete encodings

// MainLog is the interface for writing logs to workflowHistorydb
type MainLog struct {
	Path      string           `json:"-"` // path to log file to write/update
	Request   *WorkflowRequest `json:"request"`
	Main      *Log             `json:"main"`
	ByProcess map[string]*Log  `json:"byProcess"`
}

func mainLog(path string, request *WorkflowRequest) *MainLog {
	log := &MainLog{
		Path:      path,
		Request:   request,
		ByProcess: make(map[string]*Log),
	}
	return log
}

func showLog(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("error opening log: %v\n", err)
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Printf("error reading log: %v\n", err)
	}
	j := &MainLog{}
	err = json.Unmarshal(b, j)
	if err != nil {
		fmt.Printf("error unmarshalling log: %v\n", err)
	}
	printJSON(j)
}

// tmp, for debugging mostly, though could/should adapt for complete error handling interface
func check(err error) {
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func (log *MainLog) write() error {
	fmt.Println("writing main log..")

	// apply/update timestamps on the main log
	// not sure if I should collect timestamps of all writes
	// or just the times of first write and latest writes
	//
	// presently only applying timestamps at the top-level workflow (i.e., engine) level
	// NOT applying timestamps to individual tasks/subworkflows, but that would be easy to do
	// just not sure if it's useful/necessary right now

	// do this BY PROCESS
	// the "engine" corresponds to the top level workflow task
	// and should not be treated specially here
	/*
		t := ts()
		if log.Engine.Created == "" {
			log.Engine.Created = t
		}
		log.Engine.LastUpdated = t
	*/

	fmt.Println("marshalling MainLog to json..")
	j, err := json.Marshal(*log)
	check(err)
	fmt.Println("writing data to file..")
	err = ioutil.WriteFile(log.Path, j, 0644)
	check(err)
	return nil
}

// Log stores the eventLog and runtime stats for a mariner component (i.e., engine or task)
// see: https://golang.org/pkg/time/
type Log struct {
	Created        string                 `json:"created,omitempty"`     // okay - timezone???
	CreatedObj     time.Time              `json:"-"`                     // okay
	LastUpdated    string                 `json:"lastUpdated,omitempty"` // okay - timezone???
	LastUpdatedObj time.Time              `json:"-"`                     // okay
	Status         string                 `json:"status"`                // okay
	Stats          Stats                  `json:"stats"`                 // TODO
	Event          EventLog               `json:"eventLog,omitempty"`    // TODO
	Input          map[string]interface{} `json:"input"`                 // TODO for workflow; okay for task
	Output         cwl.Parameters         `json:"output"`                // okay
}

func logger() *Log {
	logger := &Log{
		Status: NOT_STARTED,
		Input:  make(map[string]interface{}),
	}
	return logger
}

// Stats holds performance stats for a given process
// recorded for tasks as well as workflows
// Runtime for a workflow is the sum of runtime of that workflow's steps
type Stats struct {
	CPU         int           `json:"cpu"`       // TODO
	Memory      int           `json:"memory"`    // TODO
	Duration    float64       `json:"duration"`  // okay - currently measured in minutes
	DurationObj time.Duration `json:"-"`         // okay
	NFailures   int           `json:"nfailures"` // TODO
	NRetries    int           `json:"nretries"`  // TODO
}

// EventLog is an event logger for a mariner component (i.e., engine or task)
type EventLog []string

// a record is "<timestamp> - <level> - <message>"
func (log EventLog) write(level, message string) {
	timestamp := ts()
	// timezone???
	record := fmt.Sprintf("%v - %v - %v", timestamp, level, message)
	log = append(log, record)
}

func (log *EventLog) infof(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.info(m)
}

func (log *EventLog) info(m string) {
	log.write(INFO, m)
}

func (log *EventLog) warnf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.warn(m)
}

func (log *EventLog) warn(m string) {
	log.write(WARNING, m)
}

func (log *EventLog) errorf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.error(m)
}

func (log *EventLog) error(m string) {
	log.write(ERROR, m)
}

// get string timestamp for right now
func ts() string {
	t := time.Now()
	s := timef(t)
	return s
}

// TODO
// should just have a function that returns (t time.Time, ts string)
// ---> return the time object AND the string timestamp for right now

// convert time object to string timestamp
// for logging purposes
// TODO timezone???
func timef(t time.Time) string {
	s := fmt.Sprintf("%v/%v/%v %v:%v:%v", t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	return s
}
