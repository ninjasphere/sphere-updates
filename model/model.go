package model

import (
	"time"
)

type Progress struct {
	Running     bool `json:"running"`
	StartTime   time.Time
	RunningTime int     `json:"runningTime"`
	Description string  `json:"description"`
	Percent     float64 `json:"percent"`
	Error       *string `json:"error,omitEmpty"`
}

func (p *Progress) UpdateRunningTime() {
	p.RunningTime = int(time.Since(p.StartTime) / time.Second)
}
