package api

import (
	"strconv"
	"time"
	"user-management-service/internal/entity"
	"user-management-service/internal/service"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

type JwtCustomClaims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// GetUserByID retrieves a user by their ID --> /users/:id
func (h *UserHandler) GetUserByID(c echo.Context) error {
	userID := c.Param("id")
	userIDInt, err := strconv.Atoi(userID)
	if err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid user ID"})
	}
	user, err := h.userService.GetUserByID(userIDInt)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, user)
}

// CreateUser creates a new user --> /users
func (h *UserHandler) CreateUser(c echo.Context) error {
	user := entity.User{}
	if err := c.Bind(&user); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	newUser, err := h.userService.CreateUser(&user)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, newUser)
}

// Login logs in a user --> /users/login
func (h *UserHandler) Login(c echo.Context) error {
	loginData := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	if err := c.Bind(&loginData); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}

	user, err := h.userService.Login(loginData.Email, loginData.Password)
	if err != nil {
		return c.JSON(401, map[string]string{"error": err.Error()})
	}

	if user == nil {
		return c.JSON(401, map[string]string{"error": "Invalid email or password"})
	}

	claims := JwtCustomClaims{
		Name:  user.Username,
		Email: user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("secret"))
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	
	return c.JSON(200, map[string]string{"token": tokenString})
}
