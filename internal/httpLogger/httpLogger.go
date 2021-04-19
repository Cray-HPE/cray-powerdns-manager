package httpLogger

import (
	"fmt"
	"go.uber.org/zap"
	"strings"
)

// Custom logger interface for retryablehttp to marshal their messages into the correct level of output.

type HTTPLogger struct {
	logger     *zap.Logger
}

func NewHTTPLogger(parentLogger *zap.Logger) *HTTPLogger {
	return &HTTPLogger{
		logger: parentLogger,
	}
}

func (logger *HTTPLogger) Printf(format string, args ...interface{}) {
	originalMessage := fmt.Sprintf(format, args...)

	if strings.HasPrefix(originalMessage, "[DEBUG]") {
		logger.logger.Debug(strings.TrimSpace(strings.TrimPrefix(originalMessage, "[DEBUG]")))
	} else if strings.HasPrefix(originalMessage, "[ERR]") {
		logger.logger.Error(strings.TrimSpace(strings.TrimPrefix(originalMessage, "[ERR]")))
	} else {
		logger.logger.Info(strings.TrimSpace(originalMessage))
	}
}
