package main

import (
	"go.uber.org/zap"
	"time"
)

func trueUpDNS() {
	logger.Info("Running true up loop at interval.", zap.Int("trueUpLoopInterval", *trueUpSleepInterval))

	defer WaitGroup.Done()

	for Running {
		logger.Debug("Running true up loop.")

		// Useful stuff.

		select {
		case <-trueUpShutdown:
			break
		case <-time.After(time.Duration(*trueUpSleepInterval) * time.Second):
			continue
		}
	}

	logger.Info("True up loop shutdown.")
}
