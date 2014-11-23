package main

import (
	"bufio"
	"io/ioutil"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/ninjasphere/go-ninja/logger"
)

var log = logger.GetLogger("updater")

type availableUpdate struct {
	Name      string
	Current   string
	Available string
}

func main() {

	lastPercent := 0.0

	updatePercentage := func(percent float64) {
		//spew.Dump(percent, lastPercent)
		if percent > lastPercent {
			lastPercent = percent
			log.Infof("Done: %f%%", lastPercent)
		}
	}

	updatePercentage(0)

	log.Infof("Updating from repositories")

	updateCache(45, 0, 25, updatePercentage)

	updates := getAvailableUpdates(updatePercentage)

	log.Infof("%d packages to update", len(updates))

	updatePercentage(30)

	distUpgrade(updates, updatePercentage)

	updatePercentage(100)

}

func updateCache(expectedLines float64, startPercent float64, totalPercent float64, update func(percent float64)) {

	if runtime.GOOS != "linux" {
		return
	}

	updateLines := 0.0

	cmd := exec.Command("apt-get", "update", "-q")

	reader, err := cmd.StdoutPipe()

	bufReader := bufio.NewReader(reader)

	err = cmd.Start()

	if err != nil {
		panic("ERROR could not spawn command." + err.Error())
	}

	for {
		_, _, err := bufReader.ReadLine()

		if err != nil {
			log.Infof("ERR:%s", err)
			break
		}

		updateLines++
		//cleaned := invalidChar.ReplaceAll([]byte(x[0:n]), []byte(""))

		update(startPercent + math.Min(totalPercent, (updateLines/expectedLines)*totalPercent))
		//	log.Infof("GOT: |%s| %t", line, isPrefix)
	}

}

func getAvailableUpdates(update func(percent float64)) []availableUpdate {
	var err error
	var available []byte

	/*ioutil.WriteFile("./available-updates.sh", []byte(`#!/bin/bash
	  apt-get -s dist-upgrade| awk -F'[][() ]+' '/^Inst/{printf "%s\t%s\t%s\n", $2,$3,$4}'`), os.FileMode(755))*/

	if runtime.GOOS == "linux" {
		available, err = exec.Command("./available-updates.sh").Output()
	} else {
		available, err = ioutil.ReadFile("./updates.txt")
	}

	if err != nil {
		panic(err)
	}

	var updates []availableUpdate

	for _, line := range strings.Split(string(available), "\n") {
		s := strings.Split(line, "\t")
		if len(s) == 3 {
			updates = append(updates, availableUpdate{s[0], s[1], s[2]})
		}
	}

	return updates
}

func distUpgrade(updates []availableUpdate, update func(percent float64)) {

	numPackages := 0.0
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		args := []string{"install", "-yy", "-q"}
		for _, update := range updates {
			if strings.Contains(update.Available, "spheramid") {
				args = append(args, update.Name)
				numPackages++
			}
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
		panic("ERROR could not spawn command." + err.Error())
	}

	pointsPerPackage := float64(10) / numPackages

	percent := 30.0

	go func() {
		for {
			line, _, err := bufReader.ReadLine()

			if err != nil {
				log.Infof("ERR:%s", err)
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

			update(math.Min(99, percent))

		}
	}()

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			// There is no plattform independent way to retrieve
			// the exit code, but the following will work on Unix
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Infof("Bad Exit Status: %d", status.ExitStatus())
				panic("Boom")
			}
		} else {
			log.Infof("cmd.Wait: %v", err)
		}
	}

}
