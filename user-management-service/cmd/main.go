package cmd

import (
	"user-management-service/internal/api"
	"user-management-service/internal/repository"
	"user-management-service/internal/service"

	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize user service
	userRepo := repository.NewUserRepository()
	userService := service.NewUserService(*userRepo)
	userHandler := api.NewUserHandler(*userService)

	// Initialize echo
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(echojwt.JWT([]byte("secret")))

	// Routes
	e.GET("/users/:id", userHandler.GetUserByID)
	e.POST("/users", userHandler.CreateUser)
	e.POST("/login", userHandler.Login)

	// Start server
	e.Start(":8080")
}
