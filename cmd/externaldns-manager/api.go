/*
 *
 *  MIT License
 *
 *  (C) Copyright 2022 Hewlett Packard Enterprise Development LP
 *
 *  Permission is hereby granted, free of charge, to any person obtaining a
 *  copy of this software and associated documentation files (the "Software"),
 *  to deal in the Software without restriction, including without limitation
 *  the rights to use, copy, modify, merge, publish, distribute, sublicense,
 *  and/or sell copies of the Software, and to permit persons to whom the
 *  Software is furnished to do so, subject to the following conditions:
 *
 *  The above copyright notice and this permission notice shall be included
 *  in all copies or substantial portions of the Software.
 *
 *  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 *  THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
 *  OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
 *  ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
 *  OTHER DEALINGS IN THE SOFTWARE.
 *
 */
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

	// True up loop control.
	apiV1.POST("/manager/jobs", func(c *gin.Context) {
		trueUpMtx.Lock()
		if trueUpInProgress {
			c.JSON(http.StatusServiceUnavailable, nil)
		} else {
			trueUpRunNow <- true
			c.JSON(http.StatusNoContent, nil)
		}
		trueUpMtx.Unlock()
	})

	// Run the router.
	srv := &http.Server{
		Addr:    ":8081",
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
