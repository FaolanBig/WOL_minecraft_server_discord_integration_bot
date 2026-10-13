package minecraft

import (
	"errors"
	"regexp"
	"strconv"
)

var playerCountPattern = regexp.MustCompile(`(?i)\bThere are\s+(\d+)\s+of a max of\s+\d+\s+players online\b`)

func ParsePlayerCount(response string) (int, error) {
	match := playerCountPattern.FindStringSubmatch(response)
	if len(match) != 2 {
		return 0, errors.New("response did not contain the expected Java Edition player count")
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, err
	}
	return count, nil
}
