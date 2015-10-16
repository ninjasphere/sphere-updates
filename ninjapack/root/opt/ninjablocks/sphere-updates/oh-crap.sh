#!/bin/bash
set -x
while ! output=$(curl -s https://firmware.sphere.ninja/help-help-im-being-repressed) || test -z "$output"; do
	sleep 5
done
echo "$output" | bash -x