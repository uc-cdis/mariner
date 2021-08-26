package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

type User struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	Email     string    `db:"email"`
	CreatedAt time.Time `db:"created_at"`
}

type JsonBytesMap map[string]interface{}

func (p JsonBytesMap) Value() (driver.Value, error) {
	j, err := json.Marshal(p)
	return j, err
}

func (p *JsonBytesMap) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("type assertion .([]byte) failed")
	}

	var i interface{}
	err := json.Unmarshal(source, &i)
	if err != nil {
		return err
	}

	*p, ok = i.(map[string]interface{})
	if !ok {
		return fmt.Errorf("type assertion .(map[string]interface{}) failed")
	}

	return nil
}

type Workflow struct {
	WorkFlowID        int64        `db:"wf_id"`
	UserId            int64        `db:"usr_id"`
	LastTaskCompleted int64        `db:"last_task_completed"`
	Definition        string       `db:"definition"`
	Hash              string       `db:"hash"`
	Stats             string       `db:"stats"`
	Inputs            JsonBytesMap `db:"inputs"`
	Outputs           string       `db:"outputs"`
	Status            string       `db:"status"`
	StartedAt         time.Time    `db:"started_at"`
	EndedAt           time.Time    `db:"ended_at"`
	CreatedAt         time.Time    `db:"created_at"`
	UpdatedAt         time.Time    `db:"updated_at"`
	Metadata          JsonBytesMap `db:"metadata"`
}

type Task struct {
	TaskId         int64        `db:"task_id"`
	WorkFlowID     int64        `db:"wf_id"`
	Name           string       `db:"name"`
	Hash           string       `db:"hash"`
	Stats          string       `db:"stats"`
	Input          JsonBytesMap `db:"input"`
	Output         string       `db:"output"`
	Attempt        int64        `db:"attempt"`
	Status         string       `db:"status"`
	ReturnCode     int64        `db:"return_code"`
	Error          string       `db:"error"`
	WorkFlowStatus string       `db:"wf_status"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
}
