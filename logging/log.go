package log

import (
	"fmt"
	"sync"
	"time"

	mariner "github.com/uc-cdis/mariner/mariner"
)

// TODO - write json encodings for all this AFTER implementing it
// so I don't constantly add/delete encodings

// MainLog is the interface for writing logs to workflowHistorydb
type MainLog struct {
	sync.RWMutex `json:"-"`
	Path         string                   `json:"path"` // tentative  - maybe can't write this - path to log file to write/update
	Request      *mariner.WorkflowRequest `json:"request"`
	Main         *Log                     `json:"main"`
	ByProcess    map[string]*Log          `json:"byProcess"`
}

// MainLogJSON gets written to workflowHistorydb
type MainLogJSON struct {
	Path      string                   `json:"path"` // tentative  - maybe can't write this - path to log file to write/update
	Request   *mariner.WorkflowRequest `json:"request"`
	Main      *Log                     `json:"main"`
	ByProcess map[string]*Log          `json:"byProcess"`
}

func InitMainLog(path string) *MainLog {
	log := &MainLog{
		Path:      path,
		Main:      logger(),
		ByProcess: make(map[string]*Log),
	}
	return log
}

// Log stores the eventLog and runtime stats for a mariner component (i.e., engine or task)
// see: https://golang.org/pkg/time/
//
// note: could log the job spec per task
// ----- or at least the task container spec
// ----- need to double check there is no sensitive info in the job spec
// ----- should be fine
// ----- for now: log container image pulled for task
type Log struct {
	Created        string                 `json:"created,omitempty"` // timezone???
	CreatedObj     time.Time              `json:"-"`
	LastUpdated    string                 `json:"lastUpdated,omitempty"` // timezone???
	LastUpdatedObj time.Time              `json:"-"`
	JobID          string                 `json:"jobID,omitempty"`
	JobName        string                 `json:"jobName,omitempty"`
	ContainerImage string                 `json:"containerImage,omitempty"`
	Status         string                 `json:"status"`
	Stats          *Stats                 `json:"stats"`
	Event          *EventLog              `json:"eventLog,omitempty"`
	Input          map[string]interface{} `json:"input"`
	Output         map[string]interface{} `json:"output"`
	Scatter        map[int]*Log           `json:"scatter,omitempty"`
}

func (r *ResourceUsage) Init() {
	r.Series = ResourceUsageSeries{}         // #race #ok
	r.SamplingPeriod = metricsSamplingPeriod // #race #ok
}

func (s *ResourceUsageSeries) Append(p ResourceUsageSamplePoint) {
	*s = append(*s, p)
}

// called when a task finishes running
func (log *Log) Finish() {
	t := time.Now()
	log.LastUpdatedObj = t
	log.LastUpdated = timef(log.LastUpdatedObj)
	log.Stats.DurationObj = t.Sub(log.CreatedObj)
	log.Stats.Duration = log.Stats.DurationObj.Seconds()
	log.Status = completed
}

// called when a task is run
func (log *Log) Start() {
	t := time.Now()
	log.CreatedObj = t
	log.Created = timef(t)
	log.LastUpdatedObj = t
	log.LastUpdated = timef(t)
	log.Status = running
}

func logger() *Log {
	logger := &Log{
		Status: notStarted,
		Input:  make(map[string]interface{}),
		Stats:  &Stats{},
		Event:  &EventLog{},
	}
	logger.Event.info("init log")
	return logger
}

// Stats holds performance stats for a given process
// recorded for tasks as well as workflows
// Runtime for a workflow is the sum of runtime of that workflow's steps
type Stats struct {
	CPUReq        ResourceRequirement `json:"cpuReq"` // in-progress
	MemoryReq     ResourceRequirement `json:"memReq"` // in-progress
	ResourceUsage ResourceUsage       `json:"resourceUsage"`
	Duration      float64             `json:"duration"`  // okay - currently measured in minutes
	DurationObj   time.Duration       `json:"-"`         // okay
	NFailures     int                 `json:"nfailures"` // TODO
	NRetries      int                 `json:"nretries"`  // TODO
}

// ResourceRequirement is for logging resource requests vs. actual usage
type ResourceRequirement struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

// ResourceUsage ..
type ResourceUsage struct {
	Series         ResourceUsageSeries `json:"data"`
	SamplingPeriod int                 `json:"samplingPeriod"`
}

// ResourceUsageSeries ..
type ResourceUsageSeries []ResourceUsageSamplePoint

// ResourceUsageSamplePoint ..
type ResourceUsageSamplePoint struct {
	CPU    int64 `json:"cpu"`
	Memory int64 `json:"mem"`
}

// EventLog is an event logger for a mariner component (i.e., engine or task)
type EventLog struct {
	sync.RWMutex
	Events []string `json:"events,omitempty"`
}

// a record is "<timestamp> - <level> - <message>"
func (log *EventLog) Write(level, message string) {
	log.Lock()
	defer log.Unlock()
	timestamp := timef(time.Now())

	record := fmt.Sprintf("%v - %v - %v", timestamp, level, message)
	log.Events = append(log.Events, record)
}

func (log *EventLog) Infof(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.info(m)
}

func (log *EventLog) info(m string) {
	log.Write(infoLogLevel, m)
}

func (log *EventLog) Warnf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.warn(m)
}

func (log *EventLog) warn(m string) {
	log.Write(warningLogLevel, m)
}

func (log *EventLog) Errorf(f string, v ...interface{}) error {
	m := fmt.Sprintf(f, v...)
	return log.error(m)
}

func (log *EventLog) error(m string) error {
	log.Write(errorLogLevel, m)
	return fmt.Errorf(m)
}

func timef(t time.Time) string {
	return t.Format("2006/01/02/ 15:04:05") // format is yyyy/mm/dd hh:mm:ss
}
