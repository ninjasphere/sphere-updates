package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type updateJob struct {
	progress   *Progress
	onProgress chan *Progress
}

func (u *updateJob) start() {

	u.progress = &Progress{
		Running:   true,
		startTime: time.Now(),
	}

	u.updateProgress(0, "Looking for updates", "")

	u.cleanup()

	err := u.setReadWrite(true)
	if err != nil {
		u.updateProgress(0, "Failed to enable disk writing", fmt.Sprintf("Error: %s", err))
		return
	}
	defer u.setReadWrite(false)

	log.Infof("Updating from repositories")

	err = u.updateCache(45, 0, 25)
	if err != nil {
		u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
		return
	}

	u.updateProgress(27, "Processing available updates", "")

	updates, err := u.getAvailableUpdates()

	if err != nil {
		u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
		return
	}

	updates = append(updates, AvailableUpdate{
		Name: "ninjasphere", // Always force it
	})

	// Check to see if our "get out of jail free" card has been played
	for _, update := range updates {
		if update.Name == "sphere-idspispopd" {
			err = u.installUpdates([]AvailableUpdate{update})

			if err != nil {
				u.updateProgress(0, "Failed running pre-install script", fmt.Sprintf("Error: %s", err))
				return
			}

			updates, err = u.getAvailableUpdates()

			if err != nil {
				u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
				return
			}
		}
	}

	u.autoRemove()

	log.Infof("%d packages to update: %v", len(updates), updates)

	if len(updates) > 0 {

		u.updateProgress(30, "Installing updates", "")

		err = u.installUpdates(updates)

		if err != nil {
			u.updateProgress(0, "Failed", fmt.Sprintf("Error: %s", err))
			return
		}
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
		u.progress.Error = &err
	}

	if description != "" {
		u.progress.Description = description
	}

	u.onProgress <- u.progress
}

func (u *updateJob) setReadWrite(writing bool) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	mode := "ro"
	if writing {
		mode = "rw"
	}

	cmd := exec.Command("mount", "-o", "remount,"+mode, "/")

	output, err := cmd.Output()
	log.Infof("Output from mount: %s", output)

	return err
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
				log.Infof("updateCache Bad Exit Status: %d", status.ExitStatus())
				return fmt.Errorf("Failed to update cache. Code: %d", status.ExitStatus())
			}
		} else {
			log.Infof("Update cache : cmd.Wait: %v", err)
		}
	}

	return nil
}

func (u *updateJob) cleanup() {
	cmd := exec.Command("dpkg", "--configure", "-a")

	output, _ := cmd.Output()

	log.Infof("Output from dpkg: %s", output)

}

func (u *updateJob) autoRemove() {
	cmd := exec.Command("apt-get", "auto-remove")

	output, _ := cmd.Output()

	log.Infof("Output from apt-get autoremove: %s", output)
}

func (u *updateJob) getAvailableUpdates() ([]AvailableUpdate, error) {
	var err error
	var available []byte

	ioutil.WriteFile("./available-updates.sh", []byte(`#!/bin/bash
	  apt-get -s dist-upgrade| awk -F'[][() ]+' '/^Inst/{printf "%s\t%s\t%s\n", $2,$3,$4}'`), os.FileMode(755))

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
		if len(s) == 3 && strings.Contains(line, "spheramid") && !strings.Contains(line, "sphere-updates") && !strings.Contains(line, "sphere-setup-assistant") {
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

var numberToInstallRegex = regexp.MustCompile(`(\d+) upgraded, (\d+) newly installed, (\d+) to remove`)

func (u *updateJob) installUpdates(updates []AvailableUpdate) error {

	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {

		os.Setenv("DEBIAN_FRONTEND", "noninteractive")
		args := []string{"install", "-yy", "-q"}
		for _, update := range updates {
			args = append(args, update.Name)
		}

		log.Infof("Running update command : apt-get %s", strings.Join(args, " "))

		cmd = exec.Command("apt-get", args...)
		cmd.Stderr = os.Stderr
	} else {
		cmd = exec.Command("cat", "./upgrade.txt")
	}

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

			log.Infof("apt: " + string(line))

			if err != nil {
				//log.Infof("ERR:%s", err)
				break
			}

			numPackages := numberToInstallRegex.FindStringSubmatch(string(line))

			if numPackages != nil {
				p := 0.0
				for _, n := range numPackages[1:] {
					i, err := strconv.ParseInt(n, 16, 10)
					if err != nil {
						log.Warningf("Failed to parse number of packages: %s", n)
					} else {
						p += float64(i)
					}
				}

				pointsPerPackage = float64(10) / p

				log.Debugf("Found nuber of packages: %f, points per package: %f", p, pointsPerPackage)
			}

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
				log.Infof("installUpdates Bad Exit Status: %d", status.ExitStatus())
				return fmt.Errorf("Failed to install updates. Code:%d", status.ExitStatus())
			}
		} else {
			log.Infof("cmd.Wait: %v", err)
		}
	}

	return nil
}
