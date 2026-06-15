package logger

import (
	"fmt"

	"go.uber.org/zap"
)

type Tlog struct {
	Log *zap.Logger
}

func Initialization() (Tlog, error) {
	log, err := zap.NewDevelopment()
	if err != nil {
		return Tlog{}, fmt.Errorf("failed zap.NewDevelopment %v", err)
	}

	return Tlog{Log: log}, nil
}
