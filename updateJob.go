package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

type updateJob struct {
	progress   *Progress
	onProgress chan *Progress
}

func (u *updateJob) start() {

	u.progress = &Progress{
		Running: true,
	}

	u.updateProgress(0, "Updating cache", "")

	log.Infof("Updating from repositories")

	err := u.updateCache(45, 0, 25)
	if err != nil {
		u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
		return
	}

	u.updateProgress(27, "Finding available updates", "")

	updates, err := u.getAvailableUpdates()

	if err != nil {
		u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
		return
	}

	log.Infof("%d packages to update: %v", len(updates))

	u.updateProgress(30, "Installing updates", "")

	err = u.installUpdates(updates)

	if err != nil {
		u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
		return
	}

	u.updateProgress(100, "Finished", "")
}

func (u *updateJob) updateProgress(percent float64, description, err string) {

	if percent == 100 || err != "" {
		u.progress.Running = false
	}

	if percent > u.progress.Percent {
		u.progress.Percent = percent
	}

	if err != "" {
		u.progress.Error = err
	}

	if description != "" {
		u.progress.Description = description
	}

	log.Debugf("Progress: %v", u.progress)

	u.onProgress <- u.progress
}

func (u *updateJob) updateCache(expectedLines float64, startPercent float64, totalPercent float64) error {

	if runtime.GOOS != "linux" {
		return nil
	}

	updateLines := 0.0

	cmd := exec.Command("apt-get", "update", "-q")

	reader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	bufReader := bufio.NewReader(reader)

	err = cmd.Start()

	if err != nil {
		return err
	}

	for {
		_, _, err := bufReader.ReadLine()

		if err != nil {
			//log.Infof("ERR:%s", err)
			break
		}

		updateLines++
		//cleaned := invalidChar.ReplaceAll([]byte(x[0:n]), []byte(""))

		u.updateProgress(startPercent+math.Min(totalPercent, (updateLines/expectedLines)*totalPercent), "", "")
		//	log.Infof("GOT: |%s| %t", line, isPrefix)
	}

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			// There is no plattform independent way to retrieve
			// the exit code, but the following will work on Unix
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Infof("Bad Exit Status: %d", status.ExitStatus())
				return fmt.Errorf("Failed to update cache. Code: %d", status.ExitStatus())
			}
		} else {
			log.Infof("Update cache : cmd.Wait: %v", err)
		}
	}

	return nil
}

func (u *updateJob) getAvailableUpdates() ([]AvailableUpdate, error) {
	var err error
	var available []byte

	/*ioutil.WriteFile("./available-updates.sh", []byte(`#!/bin/bash
	  apt-get -s dist-upgrade| awk -F'[][() ]+' '/^Inst/{printf "%s\t%s\t%s\n", $2,$3,$4}'`), os.FileMode(755))*/

	cmd := exec.Command("./available-updates.sh")

	if runtime.GOOS == "linux" {
		available, err = cmd.Output()
	} else {
		available, err = ioutil.ReadFile("./updates.txt")
	}

	if err != nil {
		return nil, fmt.Errorf("Failed to get updatable packages. Error: %s", err)
	}

	var updates []AvailableUpdate

	for _, line := range strings.Split(string(available), "\n") {
		s := strings.Split(line, "\t")
		if len(s) == 3 && strings.Contains(line, "spheramid") {
			updates = append(updates, AvailableUpdate{s[0], s[1], s[2]})
		}
	}

	if runtime.GOOS == "linux" {
		if err := cmd.Wait(); err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				// The program has exited with an exit code != 0
				// There is no plattform independent way to retrieve
				// the exit code, but the following will work on Unix
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					log.Infof("Bad Exit Status: %d", status.ExitStatus())
					return nil, fmt.Errorf("Failed to get updatable packages. Code: %d", status.ExitStatus())
				}
			} else {
				log.Infof("Get updatable : cmd.Wait: %v", err)
			}
		}
	}

	return updates, nil
}

func (u *updateJob) installUpdates(updates []AvailableUpdate) error {

	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		args := []string{"install", "-yy", "-q"}
		for _, update := range updates {
			args = append(args, update.Name)
		}

		cmd = exec.Command("apt-get", args...)
	} else {
		cmd = exec.Command("cat", "./upgrade.txt")
	}

	cmd.Env = []string{"DEBIAN_FRONTEND=noninteractive"}

	reader, err := cmd.StdoutPipe()

	bufReader := bufio.NewReader(reader)

	err = cmd.Start()

	if err != nil {
		return err
	}

	pointsPerPackage := float64(10) / float64(len(updates))

	percent := 30.0

	go func() {
		for {
			line, _, err := bufReader.ReadLine()

			if err != nil {
				//log.Infof("ERR:%s", err)
				break
			}

			log.Infof("Line: " + string(line))

			if strings.HasPrefix(string(line), "Reading package lists") {
				percent = percent + 5.0
			}

			if strings.HasPrefix(string(line), "Building dependency tree") {
				percent = percent + 5.0
			}

			if strings.HasPrefix(string(line), "Reading state information") {
				percent = percent + 5.0
			}

			if strings.HasPrefix(string(line), "Need to get") {
				percent = percent + 5.0
			}

			if strings.HasPrefix(string(line), "Get:") {
				percent = percent + (pointsPerPackage * float64(2))
			}

			if strings.HasPrefix(string(line), "Unpacking ") {
				percent = percent + (pointsPerPackage * float64(2))
			}

			if strings.HasPrefix(string(line), "Setting up ") {
				percent = percent + pointsPerPackage
			}

			u.updateProgress(math.Min(99, percent), "Installing updates", "")

		}
	}()

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			// There is no plattform independent way to retrieve
			// the exit code, but the following will work on Unix
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Infof("Bad Exit Status: %d", status.ExitStatus())
				return fmt.Errorf("Failed to install updates. Code:%d", status.ExitStatus())
			}
		} else {
			log.Infof("cmd.Wait: %v", err)
		}
	}

	return nil
}