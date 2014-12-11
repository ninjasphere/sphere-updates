#!/bin/bash
	  apt-get -s dist-upgrade| awk -F'[][() ]+' '/^Inst/{printf "%s\t%s\t%s\n", $2,$3,$4}'