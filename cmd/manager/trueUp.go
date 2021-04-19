package main

import (
	"go.uber.org/zap"
	"time"
)

func trueUpDNS() {
	logger.Info("Running true up loop at interval.", zap.Int("trueUpLoopInterval", *trueUpSleepInterval))

	defer WaitGroup.Done()

	for Running {
		trueUpMtx.Lock()
		trueUpInProgress = true
		trueUpMtx.Unlock()

		logger.Debug("Running true up loop.")

		// Useful stuff.
		time.Sleep(5 * time.Second)

		trueUpMtx.Lock()
		trueUpInProgress = false
		trueUpMtx.Unlock()

		select {
		case <-trueUpShutdown:
			break
		case <-trueUpRunNow:
			// For those impatient type.
			continue
		case <-time.After(time.Duration(*trueUpSleepInterval) * time.Second):
			continue
		}
	}

	logger.Info("True up loop shutdown.")
}
