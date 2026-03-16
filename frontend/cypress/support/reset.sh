#!/bin/bash

pkill -9 listpocket
 cd ../
./listpocket --install --yes
./listpocket > /dev/null 2>/dev/null &
