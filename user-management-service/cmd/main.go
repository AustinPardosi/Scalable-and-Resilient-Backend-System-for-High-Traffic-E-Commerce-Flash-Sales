package cmd

import (
	"user-management-service/internal/api"
	"user-management-service/internal/service"

	"github.com/labstack/echo/v4"
)

func main() {
	// Initialize user service
	userService := service.NewUserService()
	userHandler := api.NewUserHandler(*userService)

	// Initialize echo
	e := echo.New()

	// Routes
	e.GET("/users/:id", userHandler.GetUserByID)
	e.POST("/users", userHandler.CreateUser)
	e.POST("/login", userHandler.Login)

	// Start server
	e.Start(":8080")
}
