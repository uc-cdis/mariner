package mariner

import (
	"time"

	_ "github.com/lib/pq"
)

type DBUserTable struct {
	ID        int
	Name      string
	Email     string
	CreatedAt time.Time
}

type DBWorkflowTable struct {
	UserId            int
	LastTaskCompleted int
	Definition        string
	Hash              string
	Stats             string
	Inputs            string `json:"inputs"`
	Outputs           string
	Status            string
	StartedAt         time.Time
	EndedAt           time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Metadata          string `json:"metadata"`
}

type DBTaskTable struct {
	WorkFlowID     int
	Name           string
	Hash           string
	Stats          string
	Input          string `json:"input"`
	Output         string
	Attempt        int
	Status         string
	ReturnCode     int
	Error          string
	WorkFlowStatus string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DBBase struct {
	DBType     string
	DBName     string
	DBHost     string
	DBUser     string
	DBPassword string
}
