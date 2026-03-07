package config

import (
	"errors"
	"strings"
)

type Position struct {
	Name      string `yaml:"name"`
	Frequency string `yaml:"frequency"`
	Section   string `yaml:"section"`
}

func GetPositionBasedOnFrequency(frequency string) (*Position, error) {
	for _, pos := range positions {
		if pos.Frequency == frequency {
			return &pos, nil
		}
	}

	return nil, errors.New("unknown position")
}

func GetPositionByName(name string) (*Position, error) {
	for _, pos := range positions {
		if strings.EqualFold(pos.Name, name) {
			return &pos, nil
		}
	}

	return nil, errors.New("unknown position")
}
