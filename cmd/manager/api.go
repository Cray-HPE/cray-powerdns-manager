package main

import (
	"github.com/gin-gonic/gin"
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
	_ = router.Run()
}
