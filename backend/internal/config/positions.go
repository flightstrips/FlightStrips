package config

import "errors"

type Position struct {
	Name      string `yaml:"name"`
	Frequency string `yaml:"frequency"`
}

func GetPositionBasedOnFrequency(frequency string) (*Position, error) {
	for _, pos := range positions {
		if pos.Frequency == frequency {
			return &pos, nil
		}
	}

	return nil, errors.New("unknown position")
}
