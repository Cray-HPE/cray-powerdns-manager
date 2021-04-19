package main

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

func setupAPI() {
	router = gin.Default()

	// Version everything.
	apiV1 := router.Group("/v1")

	// Liveness/readiness probes.
	apiV1.GET("/liveness", func(c *gin.Context) {
		c.JSON(http.StatusNoContent, nil)
	})
	apiV1.GET("/readiness", func(c *gin.Context) {
		c.JSON(http.StatusNoContent, nil)
	})

	// Run the router.
	srv := &http.Server{
		Addr: ":8080",
		Handler: router,
	}

	go func() {
		defer WaitGroup.Done()

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Panic("Unable to start API server!", zap.Error(err))
		}

		logger.Info("API Server shutdown.")
	}()

	logger.Info("API server started.")

	APIServer = srv
}
