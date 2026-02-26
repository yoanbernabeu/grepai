package rpg

import "errors"

const CurrentRPGIndexVersion = 2

var ErrRPGIndexOutdated = errors.New("rpg index outdated")
