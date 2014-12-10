package main

import (
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/config"
	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/go-ninja/model"

	ledmodel "github.com/ninjasphere/sphere-go-led-controller/model"
)

var log = logger.GetLogger("updater")

func main() {

	conn, err := ninja.Connect("sphere-updates")

	if err != nil {
		log.FatalErrorf(err, "Failed to connect to mqtt")
	}

	ledService := conn.GetServiceClient("$node/" + config.Serial() + "/led-controller")

	service := &UpdatesService{
		job: &updateJob{
			progress:   &Progress{},
			onProgress: make(chan *Progress, 0),
		},
	}

	go func() {
		for {
			progress := <-service.job.onProgress

			//log.Infof("Progress: %v", progress)
			progress.updateRunningTime()
			service.sendEvent("progress", progress)
			if progress.Percent == 100 {
				service.sendEvent("finished", progress.Error)

				if progress.Error == nil {
					ledService.Call("displayIcon", "update-succeeded.gif", nil, 0)
				} else {
					ledService.Call("displayIcon", "update-failed.gif", nil, 0)
				}

			} else {

				ledService.Call("displayUpdateProgress", ledmodel.DisplayUpdateProgress{
					Progress: float64(progress.Percent) / float64(100),
				}, nil, 0)

			}

		}
	}()

	conn.MustExportService(service, "$node/"+config.Serial()+"/updates", &model.ServiceAnnouncement{
		Schema: "/service/updates",
	})

	if strings.Contains(strings.Join(os.Args, ""), "start") {
		go func() {
			time.Sleep(time.Second * 2)
			log.Infof("Starting update automatically")
			service.Start()
		}()
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, os.Kill)
	log.Infof("Got signal: %v", <-s)
}

type UpdatesService struct {
	job       *updateJob
	sendEvent func(event string, payload interface{}) error
}

type AvailableUpdate struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Available string `json:"available"`
}

type Progress struct {
	Running     bool `json:"running"`
	startTime   time.Time
	RunningTime int     `json:"runningTime"`
	Description string  `json:"description"`
	Percent     float64 `json:"percent"`
	Error       *string `json:"error,omitEmpty"`
}

func (p *Progress) updateRunningTime() {
	p.RunningTime = int(time.Since(p.startTime) / time.Second)
}

func (s *UpdatesService) Start() (*bool, error) {

	if s.job.progress.Running {
		x := false
		return &x, nil
	}

	go s.job.start()
	s.sendEvent("started", nil)

	x := true
	return &x, nil
}

func (s *UpdatesService) GetAvailable() (*[]AvailableUpdate, error) {
	updates, err := s.job.getAvailableUpdates()
	return &updates, err
}

func (s *UpdatesService) GetProgress() (*Progress, error) {
	s.job.progress.updateRunningTime()
	return s.job.progress, nil
}

func (s *UpdatesService) SetEventHandler(sendEvent func(event string, payload interface{}) error) {
	s.sendEvent = sendEvent
}
